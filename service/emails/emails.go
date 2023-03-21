package emails

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mikeydub/go-gallery/emails"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

func init() {
	env.RegisterValidation("EMAILS_HOST", []string{"required"})
}

func VerifyEmail(ctx context.Context, token string) (emails.VerifyEmailOutput, error) {
	input := emails.VerifyEmailInput{
		JWT: token,
	}
	body, err := json.Marshal(input)
	if err != nil {
		return emails.VerifyEmailOutput{}, err
	}

	buf := bytes.NewBuffer(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/verify", env.GetString(ctx, "EMAILS_HOST")), buf)
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

func PreverifyEmail(ctx context.Context, email persist.Email, source string) (emails.PreverifyEmailOutput, error) {
	var result emails.PreverifyEmailOutput

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/preverify?email=%s&source=%s", env.GetString(ctx, "EMAILS_HOST"), email, source), nil)
	if err != nil {
		return result, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return result, util.GetErrFromResp(resp)
	}

	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return result, err
	}

	return result, nil
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/unsubscribe", env.GetString(ctx, "EMAILS_HOST")), buf)
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/subscriptions", env.GetString(ctx, "EMAILS_HOST")), buf)
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/send/verification", env.GetString(ctx, "EMAILS_HOST")), buf)
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
