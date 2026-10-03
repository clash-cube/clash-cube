package profiles

import (
	"errors"
	"net/url"
	"strings"
)

// ImportRequest pre-fills the import form. Parsing never downloads or activates
// a profile; an external link still needs the user's confirmation in the app.
type ImportRequest struct {
	URL   string `json:"url"`
	Name  string `json:"name"`
	Error string `json:"error,omitempty"`
}

// ParseImportLink accepts Clash's install-config protocol. Error messages omit
// the input because subscription URLs often contain credentials.
func ParseImportLink(raw string) (ImportRequest, error) {
	invalid := errors.New("Invalid Clash import link")
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "clash") || !strings.EqualFold(u.Host, "install-config") ||
		(u.Path != "" && u.Path != "/") || u.User != nil || u.Fragment != "" {
		return ImportRequest{}, invalid
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(q["url"]) != 1 || len(q["name"]) > 1 {
		return ImportRequest{}, invalid
	}
	address := strings.TrimSpace(q.Get("url"))
	subscription, err := url.Parse(address)
	if err != nil || subscription.Hostname() == "" || (subscription.Scheme != "https" && subscription.Scheme != "http") {
		return ImportRequest{}, invalid
	}
	return ImportRequest{URL: address, Name: strings.TrimSpace(q.Get("name"))}, nil
}
