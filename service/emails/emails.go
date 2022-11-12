package emails

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mikeydub/go-gallery/emails"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
	"github.com/spf13/viper"
)

func VerifyEmail(ctx context.Context, token string) (emails.VerifyEmailOutput, error) {
	input := emails.VerifyEmailInput{
		JWT: token,
	}
	body, err := json.Marshal(input)
	if err != nil {
		return emails.VerifyEmailOutput{}, err
	}

	buf := bytes.NewBuffer(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/verify", viper.GetString("EMAILS_HOST")), buf)
	if err != nil {
		return emails.VerifyEmailOutput{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return emails.VerifyEmailOutput{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return emails.VerifyEmailOutput{}, util.GetErrFromResp(resp)
	}

	var output emails.VerifyEmailOutput
	err = json.NewDecoder(resp.Body).Decode(&output)
	if err != nil {
		return emails.VerifyEmailOutput{}, err
	}

	return output, nil
}

func UnsubscribeByJWT(ctx context.Context, jwt string, unsubTypes []model.EmailUnsubscriptionType) error {
	input := emails.UnsubInput{
		JWT:    jwt,
		Unsubs: unsubTypes,
	}

	body, err := json.Marshal(input)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/unsubscribe", viper.GetString("EMAILS_HOST")), buf)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return util.GetErrFromResp(resp)
	}

	return nil
}

func UpdateUnsubscriptionsByUserID(ctx context.Context, userID persist.DBID, unsubTypes persist.EmailUnsubscriptions) error {
	input := emails.UpdateSubscriptionsTypeInput{
		UserID: userID,
		Unsubs: unsubTypes,
	}

	body, err := json.Marshal(input)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/subscriptions", viper.GetString("EMAILS_HOST")), buf)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return util.GetErrFromResp(resp)
	}

	return nil
}

func RequestVerificationEmail(ctx context.Context, userID persist.DBID) error {
	input := emails.VerificationEmailInput{
		UserID: userID,
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/send/verification", viper.GetString("EMAILS_HOST")), buf)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return util.GetErrFromResp(resp)
	}

	return nil
}
