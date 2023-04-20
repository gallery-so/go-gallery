package main

import (
	"context"
	"encoding/json"
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

func listModels(ctx context.Context, client *openai.Client) ([]openai.Model, error) {
	models, err := client.ListModels(ctx)
	if err != nil {
		return nil, err
	}

	return models.Models, nil
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

func cancelFineTune(ctx context.Context, client *openai.Client, fineTuneID string) error {
	_, err := client.CancelFineTune(ctx, fineTuneID)
	if err != nil {
		return err
	}

	return nil
}

func listFiles(ctx context.Context, client *openai.Client) ([]openai.File, error) {
	files, err := client.ListFiles(ctx)
	if err != nil {
		return nil, err
	}

	return files.Files, nil
}

func main() {
	ctx := context.Background()

	apiKey := os.Getenv("OPENAI_API_KEY")
	client := openai.NewClient(apiKey)

	// // Upload a file
	// filePath := "./service/openai/gallery-generation_prepared.jsonl"
	// purpose := "fine-tune"
	// fileName := "gallery-generation_prepared.jsonl"
	// fileID, err := uploadFile(ctx, client, fileName, filePath, purpose)
	// if err != nil {
	// 	fmt.Printf("Error uploading file: %v\n", err)
	// 	return
	// }

	// fmt.Println("File uploaded successfully")

	// // Create a fine-tuned model
	// modelID := "curie"
	// trainingFileID := fileID
	// fineTunedModelID, err := createFineTunedModel(ctx, client, modelID, trainingFileID)
	// if err != nil {
	// 	fmt.Printf("Error creating fine-tuned model: %v\n", err)
	// 	return
	// }

	// fmt.Println("Fine-tuned model created successfully")

	// fis, err := listFiles(ctx, client)
	// if err != nil {
	// 	fmt.Printf("Error listing files: %v\n", err)
	// 	return
	// }

	// for _, fi := range fis {
	// 	fmt.Printf("File: %s, ID: %s \n", fi.FileName, fi.ID)
	// }

	// mis, err := listModels(ctx, client)
	// if err != nil {
	// 	fmt.Printf("Error listing models: %v\n", err)
	// 	return
	// }

	// for _, mi := range mis {
	// 	fmt.Printf("Model: %s, ID: %s \n", mi.Object, mi.ID)
	// }

	fts, err := listFineTunes(ctx, client)
	if err != nil {
		fmt.Printf("Error listing fine-tunes: %v\n", err)
		return
	}

	for _, ft := range fts {

		asJSON, err := json.MarshalIndent(ft, "", "  ")
		if err != nil {
			fmt.Printf("Error marshaling fine-tune: %v\n", err)
			return
		}
		fmt.Println(string(asJSON))

		ftes, err := listFineTuneEvents(ctx, client, ft.ID)
		if err != nil {
			fmt.Printf("Error listing fine-tune events: %v\n", err)
			return
		}

		for _, fte := range ftes {
			asJSON, err := json.MarshalIndent(fte, "", "  ")
			if err != nil {
				fmt.Printf("Error marshaling fine-tune event: %v\n", err)
				return
			}
			fmt.Println(string(asJSON))
		}
	}

	// Chat with the model
	prompt := "Organize my NFTs into collections based on the NFT's collection|2,Autoglyph #1,Autoglyphs;5,Autoglyph #22,Autoglyphs;19,Autoglyph #523,Autoglyphs;23,Bridge #2,The Bridges;50,Bridge #51,The Bridges"
	reply, err := chatWithModel(ctx, client, fts[0].FineTunedModel, prompt)
	if err != nil {
		fmt.Printf("Error chatting with model: %v\n", err)
		return
	}

	fmt.Printf("Model response: %s\n", reply)
}
