package validate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/graphql/model"
	"github.com/mikeydub/go-gallery/service/persist"
	"golang.org/x/exp/slices"

	"github.com/go-playground/validator/v10"
	"github.com/microcosm-cc/bluemonday"
)

var bannedUsernames = map[string]bool{
	"password":      true,
	"auth":          true,
	"welcome":       true,
	"edit":          true,
	"404":           true,
	"nuke":          true,
	"account":       true,
	"settings":      true,
	"artists":       true,
	"artist":        true,
	"collections":   true,
	"collection":    true,
	"community":     true,
	"nft":           true,
	"members":       true,
	"nfts":          true,
	"bookmarks":     true,
	"messages":      true,
	"guestbook":     true,
	"notifications": true,
	"explore":       true,
	"analytics":     true,
	"gallery":       true,
	"investors":     true,
	"team":          true,
	"faq":           true,
	"info":          true,
	"about":         true,
	"contact":       true,
	"terms":         true,
	"privacy":       true,
	"help":          true,
	"support":       true,
	"feed":          true,
	"feeds":         true,
	"membership":    true,
	"careers":       true,
	"maintenance":   true,
	"home":          true,
	"shop":          true,
	"chain":         true,
	"profile":       true,
	"verify":        true,
	"universal":     true,
	"search":        true,
	"unsubscribe":   true,
}

var alphanumericUnderscoresPeriodsRegex = regexp.MustCompile("^[\\w.]*$")

// SanitizationPolicy is a policy for sanitizing user input
var SanitizationPolicy = bluemonday.UGCPolicy()

func WithCustomValidators() *validator.Validate {
	v := validator.New()
	RegisterCustomValidators(v)
	return v
}

func RegisterCustomValidators(v *validator.Validate) {
	v.RegisterValidation("eth_addr", EthValidator)
	v.RegisterValidation("nonce", NonceValidator)
	v.RegisterValidation("signature", SignatureValidator)
	v.RegisterValidation("username", UsernameValidator)
	v.RegisterValidation("sorted_asc", SortedAscValidator)
	v.RegisterValidation("chain", ChainValidator)
	v.RegisterValidation("role", IsValidRole)
	v.RegisterValidation("created_collections", CreatedCollectionsValidator)
	v.RegisterAlias("collection_name", "max=200")
	v.RegisterAlias("collection_note", "max=600")
	v.RegisterAlias("token_note", "max=1200")
	v.RegisterAlias("bio", "max=600")
	v.RegisterAlias("caption", "max=600")

	v.RegisterStructValidation(ChainAddressValidator, persist.ChainAddress{})
	v.RegisterStructValidation(ConnectionPaginationParamsValidator, ConnectionPaginationParams{})
	v.RegisterStructValidation(CollectionTokenSettingsParamsValidator, CollectionTokenSettingsParams{})
	v.RegisterStructValidation(EventValidator, coredb.Event{})
}

var IsValidRole validator.Func = func(fl validator.FieldLevel) bool {
	role := persist.Role(fl.Field().String())
	return role == persist.RoleAdmin || role == persist.RoleBetaTester
}

func ChainAddressValidator(sl validator.StructLevel) {
	chainAddress := sl.Current().Interface().(persist.ChainAddress)

	address := chainAddress.Address()
	chain := chainAddress.Chain()

	// TODO: At some point in the future, validate the address based on its chain type.
	if len(address) == 0 {
		sl.ReportError(address, "Address", "Address", "required", "")
	}

	if chain < 0 || chain > persist.MaxChainValue {
		sl.ReportError(chain, "Chain", "Chain", "valid_chain_type", "")
	}
}

func EventValidator(sl validator.StructLevel) {
	event := sl.Current().Interface().(coredb.Event)

	if event.Action == "" {
		sl.ReportError(event.Action, "Action", "Action", "required", "")
	}

}

type ConnectionPaginationParams struct {
	First *int
	Last  *int
}

func ConnectionPaginationParamsValidator(sl validator.StructLevel) {
	pageArgs := sl.Current().Interface().(ConnectionPaginationParams)

	// must specify some sort of limit
	if pageArgs.First == nil && pageArgs.Last == nil {
		sl.ReportError(pageArgs.First, "First", "First", "required_without", "firstorlast")
		sl.ReportError(pageArgs.Last, "Last", "Last", "required_without", "firstorlast")
	}

	// can lead to confusing results if both are specified
	if pageArgs.First != nil && pageArgs.Last != nil {
		sl.ReportError(pageArgs.First, "First", "First", "excluded_with", "firstandlast")
		sl.ReportError(pageArgs.Last, "Last", "Last", "excluded_with", "firstandlast")
	}
}

// CollectionTokenSettingsParams are args passed to collection create and update functions that are meant to be validated together
type CollectionTokenSettingsParams struct {
	Tokens        []persist.DBID                                   `json:"tokens"`
	TokenSettings map[persist.DBID]persist.CollectionTokenSettings `json:"token_settings"`
}

// CollectionTokenSettingsParamsValidator checks that the input CollectionTokenSettingsParams struct is valid
func CollectionTokenSettingsParamsValidator(sl validator.StructLevel) {
	settings := sl.Current().Interface().(CollectionTokenSettingsParams)

	for settingTokenID := range settings.TokenSettings {
		var exists bool

		for _, tokenID := range settings.Tokens {
			if settingTokenID == tokenID {
				exists = true
				break
			}
		}

		if !exists {
			sl.ReportError(settingTokenID, fmt.Sprintf("TokenSettings[%s]", settingTokenID), "token_settings", "exclude", "")
		}
	}
}

// EthValidator validates ethereum addresses
var EthValidator validator.Func = func(fl validator.FieldLevel) bool {
	addr := fl.Field().String()
	if addr == "" {
		return true
	}
	return len(addr) == 42 && strings.HasPrefix(addr, "0x")
}

// SignatureValidator validates ethereum wallet signed messages
var SignatureValidator validator.Func = func(fl validator.FieldLevel) bool {
	sig := fl.Field().String()
	if sig == "" {
		return true
	}
	return len(sig) >= 80 && len(sig) <= 200
}

// NonceValidator validates nonces generated by the app
var NonceValidator validator.Func = func(fl validator.FieldLevel) bool {
	nonce := fl.Field().String()
	if nonce == "" {
		return true
	}
	return len(nonce) >= 10 && len(nonce) <= 150
}

// UsernameValidator ensures that usernames are not reserved, are alphanumeric with the exception of underscores and periods, and do not contain consecutive periods or underscores
var UsernameValidator validator.Func = func(fl validator.FieldLevel) bool {
	s := fl.Field().String()
	if s == "" {
		return true
	}
	if _, ok := bannedUsernames[s]; ok {
		return false
	}
	return len(s) >= 2 && len(s) <= 50 &&
		alphanumericUnderscoresPeriodsRegex.MatchString(s) &&
		!consecutivePeriodsOrUnderscores(s)
}

// SortedAscValidator validates that the array is sorted in ascending order.
var SortedAscValidator validator.Func = func(fl validator.FieldLevel) bool {
	if s, ok := fl.Field().Interface().([]int); ok {
		return sort.IntsAreSorted(s)
	}
	return false
}

// CreatedCollectionsValidator validates that the create collection input has valid given IDs
var CreatedCollectionsValidator validator.Func = func(fl validator.FieldLevel) bool {
	if s, ok := fl.Field().Interface().([]*model.CreateCollectionInGalleryInput); ok {
		return !slices.ContainsFunc(s, func(l *model.CreateCollectionInGalleryInput) bool {
			return l.GivenID == ""
		})
	}
	return true
}

// ChainValidator ensures the specified Chain is one we support
var ChainValidator validator.Func = func(fl validator.FieldLevel) bool {
	chain := fl.Field().Int()
	return chain >= 0 && chain <= int64(persist.MaxChainValue)
}

func consecutivePeriodsOrUnderscores(s string) bool {
	for i, r := range s {
		if r == '.' || r == '_' {
			if i > 0 && (rune(s[i-1]) == '.' || rune(s[i-1]) == '_') {
				return true
			}
			if i < len(s)-1 && (rune(s[i+1]) == '.' || rune(s[i+1]) == '_') {
				return true
			}
		}
	}
	return false
}
