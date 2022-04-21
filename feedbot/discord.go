package feedbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/viper"
)

type errFailedToPostMessage struct {
	err error
}

func (e errFailedToPostMessage) Retryable() bool {
	return true
}

func (e errFailedToPostMessage) Error() string {
	return fmt.Sprintf("failed to send message: %s", e.err)
}

func createMessage(content string) ([]byte, error) {
	return json.Marshal(map[string]interface{}{"content": content, "tts": false})
}

func prepareRequest(ctx context.Context, body []byte) (*http.Request, error) {
	url := fmt.Sprintf("%s/channels/%s/messages", viper.GetString("DISCORD_API"), viper.GetString("CHANNEL_ID"))
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bot "+viper.GetString("BOT_TOKEN"))
	req.Header.Set("User-Agent", viper.GetString("AGENT_NAME"))
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func sendMessage(ctx context.Context, message []byte) error {
	fmt.Println("sending %s to discord", message)
	return nil
	// client := http.Client{}
	// req, err := prepareRequest(ctx, message)
	// if err != nil {
	// 	return err
	// }
	// resp, err := client.Do(req)
	// if err != nil {
	// 	return errFailedToPostMessage{err}
	// }
	// defer resp.Body.Close()
	// body, err := ioutil.ReadAll(resp.Body)
	// if err != nil {
	// 	return err
	// }
	// if resp.StatusCode != http.StatusOK {
	// 	return errors.New(string(body))
	// }
	// return nil
}
