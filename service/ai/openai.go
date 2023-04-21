package ai

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	openai "github.com/sashabaranov/go-openai"
)

func init() {
	env.RegisterValidation("OPENAI_API_KEY", "required")
	env.RegisterValidation("OPENAI_MODEL_ID", "required")
}

type ConversationClient struct {
	client     *openai.Client
	queries    *coredb.Queries
	pseudoLang []byte
}

func NewConversationClient(queries *coredb.Queries) *ConversationClient {

	bs, err := os.ReadFile("./service/ai/psuedo-lang.md")
	if err != nil {
		panic(fmt.Sprintf("failed to read pseudo-lang file: %v", err))
	}
	return &ConversationClient{
		client:     openai.NewClient(env.GetString("OPENAI_API_KEY")),
		queries:    queries,
		pseudoLang: bs,
	}
}

func listFineTunes(ctx context.Context, client *openai.Client) ([]openai.FineTune, error) {
	fineTunings, err := client.ListFineTunes(ctx)
	if err != nil {
		return nil, err
	}

	return fineTunings.Data, nil
}

func listFineTuneEvents(ctx context.Context, client *openai.Client, fineTuneID string) ([]openai.FineTuneEvent, error) {
	fineTuneEvents, err := client.ListFineTuneEvents(ctx, fineTuneID)
	if err != nil {
		return nil, err
	}

	return fineTuneEvents.Data, nil
}

func (c *ConversationClient) GalleryConverse(ctx context.Context, prompt string, userID persist.DBID, currentConversationID *persist.DBID) ([]PseudoCollection, persist.GivenIDs, persist.DBID, error) {

	var curConversation coredb.Conversation
	var newConversation bool
	if currentConversationID != nil {
		var err error
		curConversation, err = c.queries.GetConversationByID(ctx, *currentConversationID)
		if err != nil {
			return nil, nil, "", err
		}
	} else {
		newConversation = true
		curConversation = coredb.Conversation{
			ID:            persist.GenerateID(),
			UserID:        userID,
			OpeningPrompt: prompt,
		}
	}
	messages := curConversation.Messages

	if len(curConversation.PsuedoTokens) == 0 {
		tokens, err := c.queries.GetTokensByUserId(ctx, userID)
		if err != nil {
			return nil, nil, "", err
		}
		contractNames := make(map[persist.DBID]string)
		for _, t := range tokens {
			if _, ok := contractNames[t.Contract]; ok {
				continue
			}
			contract, err := c.queries.GetContractByID(ctx, t.Contract)
			if err != nil {
				return nil, nil, "", err
			}
			contractNames[t.Contract] = contract.Name.String
		}
		curConversation.PsuedoTokens, curConversation.GivenIds, err = tokensToPsuedoTokens(tokens, contractNames)
		if err != nil {
			return nil, nil, "", err
		}
	}

	if len(messages) == 0 {
		messages = append(curConversation.Messages, openai.ChatCompletionMessage{
			Role:    "user",
			Content: fmt.Sprintf("Using the previous instructions, generate a code only response for the given prompt: %s|%s", prompt, curConversation.PsuedoTokens),
		})
		logger.For(ctx).Debugf("%s|%s", prompt, curConversation.PsuedoTokens)
	} else {
		messages = append(curConversation.Messages, openai.ChatCompletionMessage{
			Role:    "user",
			Content: fmt.Sprintf("%s (code only response)", prompt),
		})
		logger.For(ctx).Debugf("%s", prompt)
	}

	resp, usedTokens, err := c.retryConversationWithChastise(ctx, userID, messages, int(curConversation.UsedTokens), 3, func(s string) error {
		var err error
		s, err = findPseudoCodeInResponse(s)
		if err != nil {
			return err
		}
		_, err = parseGalleryPseudoCode(s)
		return err
	})
	if err != nil {
		return nil, nil, "", err
	}

	curConversation.UsedTokens = int32(usedTokens)

	resp, err = findPseudoCodeInResponse(resp)
	if err != nil {
		return nil, nil, "", err
	}

	curConversation.CurrentState = resp

	logger.For(ctx).Debugf("current state: %s", resp)

	psuedoCollections, err := parseGalleryPseudoCode(resp)
	if err != nil {
		return nil, nil, "", err
	}

	messages = append(messages, openai.ChatCompletionMessage{
		Role:    "assistant",
		Content: resp,
	})

	curConversation.Messages = messages

	if newConversation {
		_, err := c.queries.InsertConversation(ctx, coredb.InsertConversationParams{
			ID:            curConversation.ID,
			UserID:        curConversation.UserID,
			Messages:      curConversation.Messages,
			OpeningPrompt: curConversation.OpeningPrompt,
			OpeningState:  curConversation.CurrentState,
			PsuedoTokens:  curConversation.PsuedoTokens,
			GivenIds:      curConversation.GivenIds,
			UsedTokens:    curConversation.UsedTokens,
		})
		if err != nil {
			return nil, nil, "", err
		}
	} else {
		err := c.queries.UpdateConversationByID(ctx, coredb.UpdateConversationByIDParams{
			ID:           curConversation.ID,
			Messages:     curConversation.Messages,
			CurrentState: curConversation.CurrentState,
			UsedTokens:   curConversation.UsedTokens,
		})
		if err != nil {
			return nil, nil, "", err
		}
	}

	return psuedoCollections, curConversation.GivenIds, curConversation.ID, nil

}

func (c *ConversationClient) retryConversationWithChastise(ctx context.Context, userID persist.DBID, messages persist.ConversationMessages, usedTokens, maxAttempts int, valid func(string) error) (string, int, error) {
	curUsedTokens := usedTokens
	for i := 0; i < maxAttempts; i++ {
		completions, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model:     env.GetString("OPENAI_MODEL_ID"),
			MaxTokens: 2500 - usedTokens,
			N:         1,
			Messages:  append([]openai.ChatCompletionMessage{{Role: "system", Content: string(c.pseudoLang)}}, messages...),
			User:      userID.String(),
		})
		if err != nil {
			return "", 0, err
		}

		if len(completions.Choices) == 0 {
			return "", 0, fmt.Errorf("no completion choices")
		}
		content := completions.Choices[0].Message.Content

		logger.For(ctx).Debugf("completion %d: %s", i, content)

		err = valid(content)
		if err == nil {
			return content, curUsedTokens + completions.Usage.TotalTokens, nil
		}
		chastise, err := c.chastiseResponse(ctx, err, i)
		if err != nil {
			return "", 0, err
		}
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    "assistant",
			Content: content,
		})
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    "system",
			Content: fmt.Sprintf("You failed to generate valid response. This is the error returned because of your failure: %s \n%s\nTry again (code only)", err, chastise),
		})

	}
	return "", 0, fmt.Errorf("max attempts reached")
}

func (c *ConversationClient) chastiseResponse(ctx context.Context, err error, attempt int) (string, error) {
	completions, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:     env.GetString("OPENAI_MODEL_ID"),
		MaxTokens: 100,
		N:         1,
		Messages: append([]openai.ChatCompletionMessage{}, openai.ChatCompletionMessage{
			Role:    "system",
			Content: fmt.Sprintf("Given the following error, generate a snarky one liner to tell off an AI for being responsible for creating an error like this after %d attempts: %s", attempt, err.Error()),
		}),
	})
	if err != nil {
		return "", err
	}

	if len(completions.Choices) > 0 {
		return completions.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("no completion choices")

}

type PseudoCollection struct {
	Name string
	Rows []PseudoRow
}

type PseudoRow struct {
	IDs []int
}

func findPseudoCodeInResponse(resp string) (string, error) {
	idx := strings.Index(resp, "{")
	if idx == -1 {
		return "", fmt.Errorf("invalid code, must begin with {")
	}

	lastIdx := strings.LastIndex(resp, "}")
	if lastIdx == -1 {
		return "", fmt.Errorf("invalid code, must end with }")
	}

	return resp[idx : lastIdx+1], nil

}

// {The Autoglyphs|[12,205,124],[4,5]}{The Doodles|[6,7]}{Feelings|[8,9,10]}
func parseGalleryPseudoCode(code string) ([]PseudoCollection, error) {

	collSplit := strings.Split(code, "}")
	var collections []PseudoCollection
	for _, coll := range collSplit {
		var rows []PseudoRow
		coll = strings.TrimSpace(coll)
		coll = strings.TrimLeft(coll, "{")
		coll = strings.TrimRight(coll, "}")
		if coll == "" {
			continue
		}
		nameAndRows := strings.Split(coll, "|")
		if len(nameAndRows) != 2 {
			return nil, fmt.Errorf("invalid collection: %s", coll)
		}
		name := nameAndRows[0]
		rowSplit := strings.Split(nameAndRows[1], "],[")
		for _, row := range rowSplit {
			row = strings.TrimLeft(row, "[")
			row = strings.TrimRight(row, "]")
			rowIDs := strings.Split(row, ",")
			var ids []int
			for _, id := range rowIDs {
				idInt, err := strconv.Atoi(id)
				if err != nil {
					return nil, fmt.Errorf("invalid row id: %s", id)
				}
				ids = append(ids, idInt)
			}
			rows = append(rows, PseudoRow{IDs: ids})
		}
		collections = append(collections, PseudoCollection{Name: name, Rows: rows})
	}
	return collections, nil
}

func tokensToPsuedoTokens(tokens []coredb.Token, contractNames map[persist.DBID]string) (string, persist.GivenIDs, error) {
	var sb strings.Builder
	givenIDs := persist.GivenIDs{}
	for i, t := range tokens {
		var spl string
		if i > 0 {
			spl = ";"
		}
		_, err := sb.WriteString(fmt.Sprintf("%s%d,%s,%s", spl, i, t.Name.String, contractNames[t.Contract]))
		if err != nil {
			return "", nil, err
		}
		givenIDs[i] = t.ID
	}

	logger.For(context.Background()).Debugf("pseudo tokens: %s", sb.String())

	return sb.String(), givenIDs, nil
}
