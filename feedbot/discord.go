package feedbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/viper"
)

type DiscordPoster struct {
	renderFromQuery func(Query) string
}

func (d *DiscordPoster) handleQuery(ctx context.Context, q Query) error {
	content := d.renderFromQuery(q)

	message, err := json.Marshal(map[string]interface{}{
		"content": content,
		"tts":     false,
	})

	if err != nil {
		return err
	}

	return sendMessage(ctx, message)
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
	fmt.Printf("sending message: %s", string(message))
	return nil
	// XXX: client := http.Client{}
	// XXX: req, err := prepareRequest(ctx, message)
	// XXX: if err != nil {
	// XXX: 	return err
	// XXX: }

	// XXX: resp, err := client.Do(req)
	// XXX: if err != nil {
	// XXX: 	return err
	// XXX: }
	// XXX: defer resp.Body.Close()

	// XXX: body, err := ioutil.ReadAll(resp.Body)
	// XXX: if err != nil {
	// XXX: 	return err
	// XXX: }

	// XXX: if resp.StatusCode != http.StatusOK {
	// XXX: 	return errors.New(string(body))
	// XXX: }

	// XXX: return nil
}
