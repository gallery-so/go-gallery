package emails

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

func init() {
	env.RegisterValidation("EMAILS_HOST", "required")
}

type VerifyEmailOutput struct {
	UserID persist.DBID  `json:"user_id"`
	Email  persist.Email `json:"email"`
}

type VerifyEmailInput struct {
	JWT string `json:"jwt" binding:"required"`
}

type PreverifyEmailInput struct {
	Email  persist.Email `form:"email" binding:"required"`
	Source string        `form:"source" binding:"required"`
}

type PreverifyEmailOutput struct {
	Result PreverifyEmailResult `json:"result"`
}

type PreverifyEmailResult int

const (
	PreverifyEmailResultInvalid PreverifyEmailResult = iota
	PreverifyEmailResultRisky
	PreverifyEmailResultValid
)

type UnsubInput struct {
	JWT    string                          `json:"jwt" binding:"required"`
	Unsubs []model.EmailUnsubscriptionType `json:"unsubscriptions" binding:"required"`
}

type ResubInput struct {
	JWT    string                          `json:"jwt" binding:"required"`
	Resubs []model.EmailUnsubscriptionType `json:"resubscriptions" binding:"required"`
}

type UpdateSubscriptionsTypeInput struct {
	UserID persist.DBID                 `json:"user_id" binding:"required"`
	Unsubs persist.EmailUnsubscriptions `json:"unsubscriptions" binding:"required"`
}

type VerificationEmailInput struct {
	UserID persist.DBID `json:"user_id" binding:"required"`
}

func VerifyEmail(ctx context.Context, token string) (VerifyEmailOutput, error) {
	input := VerifyEmailInput{
		JWT: token,
	}
	body, err := json.Marshal(input)
	if err != nil {
		return VerifyEmailOutput{}, err
	}

	buf := bytes.NewBuffer(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/verify", env.GetString("EMAILS_HOST")), buf)
	if err != nil {
		return VerifyEmailOutput{}, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return VerifyEmailOutput{}, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return VerifyEmailOutput{}, util.GetErrFromResp(resp)
	}

	var output VerifyEmailOutput
	err = json.NewDecoder(resp.Body).Decode(&output)
	if err != nil {
		return VerifyEmailOutput{}, err
	}

	return output, nil
}

func PreverifyEmail(ctx context.Context, email persist.Email, source string) (PreverifyEmailOutput, error) {
	var result PreverifyEmailOutput

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/preverify?email=%s&source=%s", env.GetString("EMAILS_HOST"), email, source), nil)
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
	input := UnsubInput{
		JWT:    jwt,
		Unsubs: unsubTypes,
	}

	body, err := json.Marshal(input)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/unsubscribe", env.GetString("EMAILS_HOST")), buf)
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
	input := UpdateSubscriptionsTypeInput{
		UserID: userID,
		Unsubs: unsubTypes,
	}

	body, err := json.Marshal(input)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/subscriptions", env.GetString("EMAILS_HOST")), buf)
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
	input := VerificationEmailInput{
		UserID: userID,
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}

	buf := bytes.NewBuffer(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/send/verification", env.GetString("EMAILS_HOST")), buf)
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
