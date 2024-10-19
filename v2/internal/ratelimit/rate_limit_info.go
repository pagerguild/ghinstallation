package ratelimit

import (
	"net/http"
	"strconv"
	"time"
)

const (
	headerRateLimitRemaining = "x-ratelimit-remaining"
	headerRateLimitUsed      = "x-ratelimit-used"
	headerRateLimitReset     = "x-ratelimit-reset"
)

// A measure of the status of rate limits for the current installation.
//
// https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api?apiVersion=2022-11-28#checking-the-status-of-your-rate-limit
type GitHubRateLimitInfo struct {
	Remaining int64 // The number of requests remaining in the current rate limit window
	Used      int64 // The number of requests you have made in the current rate limit window
	Reset     int64 // The time at which the current rate limit window resets, in UTC epoch seconds
}

// When returns the time.Time when the rate limit window will reset.
func (rateLimit GitHubRateLimitInfo) When() time.Time {
	return time.Unix(rateLimit.Reset, 0)
}

func (rateLimit GitHubRateLimitInfo) Valid() bool {
	return rateLimit.Reset != 0 || rateLimit.Remaining != 0 || rateLimit.Used != 0
}

// TimeToReset returns the time.Duration until the window will reset.
func (rateLimit GitHubRateLimitInfo) TimeToReset() time.Duration {
	return time.Until(rateLimit.When())
}

func NewGitHubRateLimitInfo(res *http.Response) GitHubRateLimitInfo {
	header := intHeader(res.Header)
	return GitHubRateLimitInfo{
		Remaining: header.GetInt64(headerRateLimitRemaining),
		Used:      header.GetInt64(headerRateLimitUsed),
		Reset:     header.GetInt64(headerRateLimitReset),
	}
}

type ErrorWithRateLimit struct {
	error
	GitHubRateLimitInfo
}

func NewErrorWithRateLimit(resp *http.Response, err error) ErrorWithRateLimit {
	return ErrorWithRateLimit{
		error:               err,
		GitHubRateLimitInfo: NewGitHubRateLimitInfo(resp),
	}
}

func (e ErrorWithRateLimit) GetError() error {
	return e.error
}

type intHeader http.Header

func (h intHeader) GetInt64(name string) int64 {
	stringVal := http.Header(h).Get(name)
	if stringVal == "" {
		return 0
	}

	intVal, err := strconv.ParseInt(stringVal, 10, 64)
	if err != nil {
		return 0
	}

	return intVal
}
