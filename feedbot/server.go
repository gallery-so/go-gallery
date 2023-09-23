package feedbot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mikeydub/go-gallery/env"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/task"
	"github.com/mikeydub/go-gallery/util"
	"github.com/shurcooL/graphql"
)

func postToDiscord(gql *graphql.Client) gin.HandlerFunc {
	discordHandler := PostRenderSender{PostRenderer: PostRenderer{gql}}
	return func(c *gin.Context) {
		message := task.FeedbotMessage{}

		if err := c.ShouldBindJSON(&message); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		err := discordHandler.RenderAndSend(c.Request.Context(), message)
		if err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("event=%s processed", message.FeedEventID)})
	}
}

func postToSlack(gql *graphql.Client) gin.HandlerFunc {
	webhookURL := env.GetString("SLACK_WEBHOOK_URL")
	webhookURL = "https://hooks.slack.com/services/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"

	if webhookURL == "" {
		return func(c *gin.Context) {
			logger.For(c).Info("slack webhook url not set, skipping")
			c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
		}
	}

	return func(c *gin.Context) {
		message := task.FeedbotSlackPostMessage{}
		if err := c.ShouldBindJSON(&message); err != nil {
			util.ErrResponse(c, http.StatusOK, err)
			return
		}

		var template slackTemplateInfo

		err := gql.Query(c, &template, map[string]any{"id": message.PostID})
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if err := template.PostOrError.Error.Message; err != "" {
			util.ErrResponse(c, http.StatusOK, fmt.Errorf(err))
			return
		}

		if len(template.PostOrError.Post.Tokens) == 0 {
			c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
			return
		}

		var postURL string

		switch e := env.GetString("ENV"); e {
		case "prod":
			postURL = "https://gallery.so/posts"
		default:
			postURL = "https://gallery-dev.vercel.app"
		}

		contextBlock := map[string]any{}
		contextBlock["type"] = "context"
		contextBlock["elements"] = make([]any, 0)
		contextBlock["elements"] = append(contextBlock["elements"].([]any), textObject(fmt.Sprintf("New Post - *%s*", template.PostOrError.Post.Tokens[0].Community.Name)))

		if pfp := template.PostOrError.Post.Author.ProfileImage.TokenProfileImage.Token.Media.Media.PreviewURLs.Thumbnail; pfp != "" {
			contextBlock["elements"] = append(contextBlock["elements"].([]any), imageObject(pfp, fmt.Sprintf("pfp of %s", template.PostOrError.Post.Author.Username)))
		} else if pfp := template.PostOrError.Post.Author.ProfileImage.EnsProfileImage.ProfileImage.PreviewURLs.Thumbnail; pfp != "" {
			contextBlock["elements"] = append(contextBlock["elements"].([]any), imageObject(pfp, fmt.Sprintf("pfp of %s", template.PostOrError.Post.Author.Username)))
		}

		contextBlock["elements"] = append(contextBlock["elements"].([]any), textObject(fmt.Sprintf("posted by *%s*", template.PostOrError.Post.Author.Username)))

		div := dividerObject()

		body := make(map[string]any)
		body["blocks"] = []any{
			contextBlock,
			div,
			map[string]any{
				"type":      "section",
				"text":      textObject(template.PostOrError.Post.Caption),
				"accessory": imageObject(template.PostOrError.Post.Tokens[0].Media.Media.PreviewURLs.Small, template.PostOrError.Post.Tokens[0].Name),
			},
			map[string]any{
				"type":      "section",
				"text":      textObject(" "),
				"accessory": linkButtonObject("View Post", fmt.Sprintf("%s/post/%s", postURL, message.PostID)),
			},
			div,
		}

		r, err := json.Marshal(body)
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		resp, err := http.DefaultClient.Post(webhookURL, "application/json", bytes.NewBuffer(r))
		if err != nil {
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		if resp.StatusCode != http.StatusOK {
			err = util.BodyAsError(resp)
			util.ErrResponse(c, http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, util.SuccessResponse{Success: true})
	}
	return nil
}

type slackTemplateInfo struct {
	PostOrError struct {
		Post struct {
			Author struct {
				Username     string
				ProfileImage struct {
					TokenProfileImage struct {
						Token struct {
							Name  string
							Media struct {
								Media struct {
									PreviewURLs struct {
										Thumbnail string
										Small     string
									} `graphql:"previewURLs"`
								} `graphql:"...on Media"`
							}
						}
					} `graphql:"...on TokenProfileImage"`
					EnsProfileImage struct {
						ProfileImage struct {
							PreviewURLs struct {
								Thumbnail string
								Small     string
							} `graphql:"previewURLs"`
						}
					} `graphql:"...on EnsProfileImage"`
				}
			}
			Caption string
			Tokens  []struct {
				Name  string
				Media struct {
					Media struct {
						PreviewURLs struct {
							Thumbnail string
							Small     string
						} `graphql:"previewURLs"`
					} `graphql:"...on Media"`
				}
				Community struct {
					Name string
				}
			}
		} `graphql:"...on Post"`
		Error struct {
			Message string
		} `graphql:"...on Error"`
	} `graphql:"postById(id: $id)"`
}

func textObject(s string) map[string]any {
	return map[string]any{"type": "mrkdwn", "text": s}
}

func imageObject(url, altText string) map[string]any {
	return map[string]any{"type": "image", "image_url": url, "alt_text": altText}
}

func dividerObject() map[string]any {
	return map[string]any{"type": "divider"}
}

func linkButtonObject(buttonText, url string) map[string]any {
	return map[string]any{
		"type": "button",
		"text": map[string]any{"type": "plain_text", "text": buttonText},
		"url":  url,
	}
}
