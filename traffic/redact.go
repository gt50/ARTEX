package traffic

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/url"
	"strings"
)

func credentialField(key string) bool {
	key = strings.ToLower(strings.NewReplacer("-", "", "_", "", " ", "").Replace(key))
	switch key {
	case "authorization", "proxyauthorization", "cookie", "setcookie", "apikey", "xapikey", "password", "passwd", "token", "accesstoken", "refreshtoken", "idtoken", "clientsecret", "secret", "jwt":
		return true
	case "auth", "session", "sessionid", "jsessionid", "sig", "signature", "credential", "credentials":
		return true
	}
	// Provider environments and custom headers commonly prefix these names,
	// e.g. OPENAI_API_KEY, AWS_SECRET_ACCESS_KEY and X-Session-Token.
	for _, suffix := range []string{"apikey", "password", "passwd", "token", "secret", "secretaccesskey", "sessionid", "signature"} {
		if strings.HasSuffix(key, suffix) {
			return true
		}
	}
	return false
}

func redactURL(input *url.URL) *url.URL {
	u := *input
	if u.User != nil {
		u.User = url.User("REDACTED")
	}
	q := u.Query()
	changed := false
	for key := range q {
		if credentialField(key) {
			q.Set(key, "[REDACTED]")
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}
	return &u
}

func redactBody(body []byte, contentType string) []byte {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var trailing any
	if decoder.Decode(&value) == nil && decoder.Decode(&trailing) == io.EOF {
		var scrub func(any) bool
		scrub = func(v any) bool {
			changed := false
			switch obj := v.(type) {
			case map[string]any:
				for key, child := range obj {
					if credentialField(key) {
						obj[key] = "[REDACTED]"
						changed = true
					} else if scrub(child) {
						changed = true
					}
				}
			case []any:
				for _, child := range obj {
					if scrub(child) {
						changed = true
					}
				}
			}
			return changed
		}
		if scrub(value) {
			if encoded, err := json.Marshal(value); err == nil {
				return encoded
			}
		}
	}
	media, _, _ := mime.ParseMediaType(contentType)
	if media == "application/x-www-form-urlencoded" {
		form, err := url.ParseQuery(string(body))
		if err != nil {
			return body
		}
		changed := false
		for key := range form {
			if credentialField(key) {
				form.Set(key, "[REDACTED]")
				changed = true
			}
		}
		if changed {
			return []byte(form.Encode())
		}
	}
	return body
}
