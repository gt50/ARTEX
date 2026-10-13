package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

//telegramTextLimit is the upper limit (number of characters) of the text field of Telegram sendMessage.
const telegramTextLimit = 4096

//telegramChannel implements Telegram Bot API.
//
//Platform features:
//- The authentication is all in the URL path (/bot<token>/sendMessage), no signature is required.
//- Parse pattern with HTML instead of MarkdownV2: MarkdownV2 requires escaping `_*[]()~`>#+-=|{}.!`
//There are 18 characters in total, if one is missing, the entire message will be rejected; HTML only needs to escape three & < >.
//- Business errors are also hidden in HTTP 200 and are judged by the ok field.
type telegramChannel struct{}

func (telegramChannel) Kind() string { return KindTelegram }

//Telegram single chat is about 1 message/second and group chat is 20 messages/minute. Take a conservative value.
func (telegramChannel) DefaultRatePerMin() int { return 20 }

//Bot Token is a complete credential; chat_id is just the recipient, not a secret (if you get it, you can't send a message without the Token).
func (telegramChannel) SecretKeys() []string { return []string{"bot_token"} }

//base_url determines which API endpoint the Token is sent to (such as a self-built reverse proxy). If you change it, you must re-state the Token.
func (telegramChannel) DestinationKeys() []string { return []string{"base_url"} }

func (telegramChannel) Validate(cfg map[string]any) error {
	if cfgString(cfg, "bot_token") == "" {
		return errors.New("Bot token is required")
	}
	if cfgString(cfg, "chat_id") == "" {
		return errors.New("Chat ID is required")
	}
	if base := cfgString(cfg, "base_url"); base != "" {
		if err := validateHTTPURL(base); err != nil {
			return fmt.Errorf("invalid API URL: %w", err)
		}
	}
	return nil
}

func (c telegramChannel) Send(ctx context.Context, cfg map[string]any, m Message) (int, error) {
	if err := c.Validate(cfg); err != nil {
		return 0, Permanent(err)
	}
	endpoint, err := telegramEndpoint(cfg)
	if err != nil {
		return 0, Permanent(err)
	}
	text, kept := telegramHTML(m)
	payload := map[string]any{
		"chat_id":                  cfgString(cfg, "chat_id"),
		"text":                     text,
		"parse_mode":               "HTML",
		"disable_web_page_preview": false,
	}
	raw, err := doJSON(ctx, "POST", endpoint, nil, payload)
	if err != nil {
		return 0, err
	}
	var res struct {
		OK          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return 0, fmt.Errorf("failed to parse Telegram response: %w (%s)", err, snippet(raw))
	}
	if res.OK {
		return kept, nil
	}
	//429 is rate limiting, and retrying after backing off is valid; the rest (400 parameter error, 401 token error, 403 blacklisted,
	//404 chat does not exist) are all configuration issues, and retrying will not heal itself.
	if res.ErrorCode == 429 {
		return 0, fmt.Errorf("Telegram rate limit: %s", res.Description)
	}
	return 0, Permanent(fmt.Errorf("Telegram returned error %d: %s", res.ErrorCode, res.Description))
}

//telegramEndpoint spells out the sendMessage address. When base_url is left blank, use the official API.
//When it is not empty, it is used to build the self-built Bot API reverse proxy (a common requirement under domestic networks).
func telegramEndpoint(cfg map[string]any) (string, error) {
	base := cfgString(cfg, "base_url")
	if base == "" {
		base = "https://api.telegram.org"
	}
	base = strings.TrimSuffix(base, "/")
	token := cfgString(cfg, "bot_token")
	raw := base + "/bot" + token + "/sendMessage"
	u, err := url.Parse(raw)
	if err != nil {
		//Non-transparent err: The address contains Bot Token, and even addr should not be echoed at this time.
		return "", fmt.Errorf("failed to build API URL (base URL: %s)", redactRequestTarget(base))
	}
	return u.String(), nil
}

//telegramHTML renders the HTML text, returning the text and the actual number of entries written (see Channel.Send).
func telegramHTML(m Message) (string, int) {
	var b strings.Builder
	b.WriteString("<b>" + telegramEscape(markdownTitle(m)) + "</b>\n")
	if m.Batch {
		//The upper limit of Telegram is the number of characters, so packaging is also measured by characters (runeSize).
		footer := ""
		if m.HomeURL != "" {
			footer = fmt.Sprintf("\n\n<a href=\"%s\">View all in ARTEX</a>", telegramEscapeAttr(m.HomeURL))
		}
		kept := packItemCount(m.Items, telegramTextLimit, telegramReservedRunes, footer, runeSize, func(it Item, idx int) string {
			return telegramBatchLine(it, idx+1)
		})
		items := m.Items[:kept]
		b.Reset()
		b.WriteString("<b>" + telegramEscape(telegramBatchTitle(m, items, len(m.Items))) + "</b>")
		for i, it := range items {
			b.WriteString("\n" + telegramEscape(telegramBatchLine(it, i+1)))
		}
		b.WriteString(footer)
		return TruncateHTML(b.String(), telegramTextLimit), kept
	}
	if len(m.Items) == 0 {
		return b.String(), 0
	}
	it := m.Items[0]
	if it.IsStatusChange() {
		b.WriteString(fmt.Sprintf("\n<b>Status change</b>: %s → %s",
			telegramEscape(StatusLabel(it.FromStatus)), telegramEscape(StatusLabel(it.ToStatus))))
	}
	if it.VulnClass != "" && it.VulnClass != it.Title() {
		b.WriteString("\n<b>Type</b>: " + telegramEscape(it.VulnClass))
	}
	if a := assetLine(it.Assets, maxAssetsShown); a != "" {
		b.WriteString("\n<b>Assets</b>: " + telegramEscape(a))
	}
	if s := OneLine(it.Summary, maxSummaryRunes); s != "" {
		b.WriteString("\n<b>Summary</b>: " + telegramEscape(s))
	}
	if it.DetailURL != "" {
		b.WriteString(fmt.Sprintf("\n\n<a href=\"%s\">View details</a>", telegramEscapeAttr(it.DetailURL)))
	}
	return TruncateHTML(b.String(), telegramTextLimit), 1
}

//telegramReservedRunes Reserved for message headers and possible truncation hints (in characters).
const telegramReservedRunes = 160

//telegramBatchLine A line in the rendering summary (not escaped, escaped uniformly by the caller).
func telegramBatchLine(it Item, idx int) string {
	if a := assetLine(it.Assets, maxAssetsShown); a != "" {
		return fmt.Sprintf("%d. %s · %s — %s", idx, SeverityLabel(it.Severity), it.Title(), a)
	}
	return fmt.Sprintf("%d. %s · %s", idx, SeverityLabel(it.Severity), it.Title())
}

//telegramBatchTitle Renders the header row of the summary message. The number of items is the number of items that **this item actually contains**.
//Rather than the total number of this batch - otherwise readers will think that the number written in the header of the message is the whole number.
func telegramBatchTitle(m Message, items []Item, total int) string {
	title := fmt.Sprintf("Finding digest · %d items", total)
	if extra := total - len(items); extra > 0 {
		title += fmt.Sprintf(" (showing the first %d items; the remaining %d continue in the next message)", len(items), extra)
	}
	if m.WindowMinutes > 0 {
		title = fmt.Sprintf("Last %d minutes · %s", m.WindowMinutes, title)
	}
	return title
}

//telegramEscape escapes the HTML text content.
//Telegram only recognizes these three entities. After escaping, existing entities such as & will be escaped twice - this is exactly
//Desired behavior: We want to display the original characters, not have the user inject HTML.
func telegramEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

//telegramEscapeAttr escapes the HTML attribute value. In addition to text escaping, you also need to deal with quotation marks——
//The quotes in URL will close the href attribute in advance, turning the following content into an injection point.
func telegramEscapeAttr(s string) string {
	s = telegramEscape(s)
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
