package feedbot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/spf13/viper"
)

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
	req, err := prepareRequest(ctx, message)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return errors.New(string(body))
	}

	return nil
}
