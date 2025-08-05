package env

import (
	"errors"
	"os"
)

type Env interface {
	GetOmsPortalApiKey() (string, error)
	GetOmsPortalApi() string
}

type Environment struct {
}

func NewEnv() *Environment {
	return &Environment{}
}

func (e *Environment) GetOmsPortalApiKey() (string, error) {
	apiToken := os.Getenv("OMS_PORTAL_API_KEY")
	if apiToken == "" {
		return "", errors.New("OMS_PORTAL_API_KEY env var required, but not set")
	}
	return apiToken, nil
}

func (e *Environment) GetOmsPortalApi() string {
	apiUrl := os.Getenv("OMS_PORTAL_API")
	if apiUrl == "" {
		return "https://oms-portal.codesphere.com/api"
	}
	return apiUrl
}
