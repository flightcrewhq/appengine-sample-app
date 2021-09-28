package util

import (
	"log"
	"os"
	"strconv"
)

const (
	EnvHeadline         = "HEADLINE"
	EnvTextColor        = "TEXT_COLOR"
	EnvConnPoolSize     = "CONN_POOL_SIZE"
	EnvMaxThreads       = "MAX_THREADS"
	EnvSelfDocs         = "SELF_DOCS"
	EnvFeedDocs         = "FEED_DOCS"
	EnvIncludeFollowers = "INCLUDE_FOLLOWERS"
	EnvTxnRetryStrat    = "TXN_RETRY_STRAT"

	EnvCloudProject   = "GOOGLE_CLOUD_PROJECT"
	EnvAppCredentials = "GOOGLE_APPLICATION_CREDENTIALS"

	FeedServiceURL = "https://feed-dot-%s.uc.r.appspot.com/%s"
	UserServiceURL = "https://user-dot-%s.uc.r.appspot.com/%s"
)

func LoadEnvString(field, defaultVal string) string {
	if v, ok := os.LookupEnv(field); ok {
		return v
	}

	log.Printf("No %s field, defaulting to %s", field, defaultVal)
	return defaultVal
}

func LoadEnvInt(field string, defaultVal int) int {
	if val, err := strconv.Atoi(LoadEnvString(field, strconv.Itoa(defaultVal))); err == nil {
		return val
	}

	log.Printf("Invalid %s field, defaulting to %d", field, defaultVal)
	return defaultVal
}

func LoadEnvBool(field string, defaultVal bool) bool {
	if val, err := strconv.ParseBool(LoadEnvString(field, strconv.FormatBool(defaultVal))); err == nil {
		return val
	}

	log.Printf("Invalid %s field, defaulting to %v", field, defaultVal)
	return defaultVal
}

func MustLoadEnvString(field string) string {
	if v := os.Getenv(field); v != "" {
		return v
	}

	log.Fatalf("No %s field, exiting.", field)
	return ""
}
