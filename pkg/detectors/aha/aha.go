package aha

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/trufflesecurity/trufflehog/v3/pkg/common"
	"github.com/trufflesecurity/trufflehog/v3/pkg/detectors"
	"github.com/trufflesecurity/trufflehog/v3/pkg/pb/detectorspb"
)

type Scanner struct {
	client *http.Client
}

// Ensure the Scanner satisfies the interface at compile time.
var _ detectors.Detector = (*Scanner)(nil)

var (
	defaultClient = common.SaneHttpClient()

	// Make sure that your group is surrounded in boundary characters such as below to reduce false positives.
	keyPat = regexp.MustCompile(detectors.PrefixRegex([]string{"aha"}) + `\b([0-9a-f]{64})\b`)
	URLPat = regexp.MustCompile(`\b([A-Za-z0-9](?:[A-Za-z0-9\-]{0,61}[A-Za-z0-9])\.aha\.io)`)
)

// Keywords are used for efficiently pre-filtering chunks.
// Use identifiers in the secret preferably, or the provider name.
func (s Scanner) Keywords() []string {
	return []string{"aha"}
}

func (s Scanner) getClient() *http.Client {
	if s.client != nil {
		return s.client
	}
	return defaultClient
}

// FromData will find and optionally verify Aha secrets in a given set of bytes.
func (s Scanner) FromData(ctx context.Context, verify bool, data []byte) (results []detectors.Result, err error) {
	dataStr := string(data)
	matches := keyPat.FindAllStringSubmatch(dataStr, -1)
	URLmatches := URLPat.FindAllStringSubmatch(dataStr, -1)

	resURLMatch := "aha.io"
	for _, URLmatch := range URLmatches {
		if len(URLmatch) != 2 {
			continue
		}
		resURLMatch = strings.TrimSpace(URLmatch[1])
	}

	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		resMatch := strings.TrimSpace(match[1])

		s1 := detectors.Result{
			DetectorType: detectorspb.DetectorType_Aha,
			Raw:          []byte(resMatch),
		}

		if verify {
			client := s.getClient()
			isVerified, verificationErr := verifyAha(ctx, client, resMatch, resURLMatch)
			s1.Verified = isVerified
			s1.SetVerificationError(verificationErr, resMatch)
		}

		// This function will check false positives for common test words, but also it will make sure the key appears 'random' enough to be a real key.
		if !s1.Verified && detectors.IsKnownFalsePositive(resMatch, detectors.DefaultFalsePositives, true) {
			continue
		}

		results = append(results, s1)
	}

	return results, nil
}

func verifyAha(ctx context.Context, client *http.Client, resMatch, resURLMatch string) (bool, error) {
	url := fmt.Sprintf("https://%s/api/v1/me", resURLMatch)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Add("Accept", "application/vnd.aha+json; version=3")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", resMatch))
	res, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()

	// https://www.aha.io/api
	if res.StatusCode == http.StatusOK {
		return true, nil
	} else if res.StatusCode < http.StatusUnauthorized || res.StatusCode > http.StatusNotFound {
		return false, fmt.Errorf("unexpected HTTP response status %d", res.StatusCode)
	}

	return false, nil
}

func (s Scanner) Type() detectorspb.DetectorType {
	return detectorspb.DetectorType_Aha
}
