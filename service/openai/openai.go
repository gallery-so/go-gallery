package main

import (
	"context"
	"fmt"
	"os"

	openai "github.com/sashabaranov/go-openai"
)

// Uploads a file to OpenAI
func uploadFile(ctx context.Context, client *openai.Client, fileName string, filePath string, purpose string) (string, error) {

	upload, err := client.CreateFile(ctx, openai.FileRequest{
		Purpose:  purpose,
		FilePath: filePath,
		FileName: fileName,
	})
	if err != nil {
		return "", err
	}

	return upload.ID, nil
}

// Creates a fine-tuned model
func createFineTunedModel(ctx context.Context, client *openai.Client, modelID string, trainingFileID string) (string, error) {
	fineTuning, err := client.CreateFineTune(ctx, openai.FineTuneRequest{
		Model:        modelID,
		TrainingFile: trainingFileID,
	})
	if err != nil {
		return "", err
	}

	return fineTuning.ID, nil
}

// Sends a message to the chat model and returns the model's reply
func chatWithModel(ctx context.Context, client *openai.Client, modelID string, prompt string) (string, error) {
	completions, err := client.CreateCompletion(ctx, openai.CompletionRequest{
		Model:     modelID,
		Prompt:    prompt,
		N:         1,
		MaxTokens: 2048,
		Stop:      []string{"\n"},
	})
	if err != nil {
		return "", err
	}

	if len(completions.Choices) > 0 {
		return completions.Choices[0].Text, nil
	}

	return "", fmt.Errorf("no completion choices")
}

func main() {
	ctx := context.Background()

	apiKey := os.Getenv("OPENAI_API_KEY")
	client := openai.NewClient(apiKey)

	// Upload a file
	filePath := "./service/openai/psuedo-gallery-code.txt"
	purpose := "fine-tuning"
	fileName := "pseudo-gallery-code.txt"
	fileID, err := uploadFile(ctx, client, fileName, filePath, purpose)
	if err != nil {
		fmt.Printf("Error uploading file: %v\n", err)
		return
	}

	// Create a fine-tuned model
	modelID := "text-gpt-3.5-turbo"
	trainingFileID := fileID
	fineTunedModelID, err := createFineTunedModel(ctx, client, modelID, trainingFileID)
	if err != nil {
		fmt.Printf("Error creating fine-tuned model: %v\n", err)
		return
	}

	// Chat with the model
	prompt := "How does photosynthesis work?"
	reply, err := chatWithModel(ctx, client, fineTunedModelID, prompt)
	if err != nil {
		fmt.Printf("Error chatting with model: %v\n", err)
		return
	}

	fmt.Printf("Model response: %s\n", reply)
}

// func main() {
// 	client := openai.NewClient("your token")
// 	messages := make([]openai.ChatCompletionMessage, 0)
// 	reader := bufio.NewReader(os.Stdin)
// 	fmt.Println("Conversation")
// 	fmt.Println("---------------------")

// 	for {
// 		fmt.Print("-> ")
// 		text, _ := reader.ReadString('\n')
// 		// convert CRLF to LF
// 		text = strings.Replace(text, "\n", "", -1)
// 		messages = append(messages, openai.ChatCompletionMessage{
// 			Role:    openai.ChatMessageRoleUser,
// 			Content: text,
// 		})

// 		resp, err := client.CreateChatCompletion(
// 			context.Background(),
// 			openai.ChatCompletionRequest{
// 				Model:    openai.GPT3Dot5Turbo,
// 				Messages: messages,
// 			},
// 		)

// 		if err != nil {
// 			fmt.Printf("ChatCompletion error: %v\n", err)
// 			continue
// 		}

// 		content := resp.Choices[0].Message.Content
// 		messages = append(messages, openai.ChatCompletionMessage{
// 			Role:    openai.ChatMessageRoleAssistant,
// 			Content: content,
// 		})
// 		fmt.Println(content)
// 	}
// }
