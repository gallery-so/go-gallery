package runtime

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"
)

const (
	method  = "GET"
	service = "managedblockchain"
	region  = "us-east-1"
)

func hmacSign(k []byte, m string) []byte {
	return hmac.New(sha256.New, k).Sum([]byte(m))
}

func getAWSSigKey(k string, t time.Time) []byte {
	date := t.Format("20060102")
	kDate := hmacSign([]byte("AWS4"+k), date)
	kRegion := hmacSign(kDate, region)
	kService := hmacSign(kRegion, service)
	signingKey := hmacSign(kService, "aws4_request")
	return signingKey
}

func createAWSSigV4Headers(t time.Time, pRuntime *Runtime) (string, string) {

	amz := t.Format("20060102T150405Z")
	canonicalURI := "/"
	canonicalQueryString := ""
	canonicalHeaders := "host:" + strings.TrimSuffix("https://", pRuntime.Config.AWSManagedBlockchainURL) + "\n" + "x-amz-date:" + amz + "\n"
	signedHeaders := "host;x-amz-date"
	payloadHash := sha256.New().Sum([]byte(""))
	canonicalRequest := method + "\n" + canonicalURI + "\n" + canonicalQueryString + "\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + string(payloadHash)
	signMethod := "AWS4-HMAC-SHA256"
	datestamp := t.Format("20060102")
	credentialScope := datestamp + "/" + region + "/" + service + "/" + "aws4_request"
	stringToSign := signMethod + "\n" + amz + "\n" + credentialScope + "\n" + canonicalRequest
	signingKey := getAWSSigKey(pRuntime.Config.AWSSecretAccessKey, t)
	sig := hmacSign(signingKey, stringToSign)
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", pRuntime.Config.AWSAccessKeyID, credentialScope, signedHeaders, string(sig))
	return amz, authHeader
}
