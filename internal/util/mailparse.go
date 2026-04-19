package util

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
)

// ParsedMail is normalized body/header output consumed by relay sender layer.
type ParsedMail struct {
	Subject   string
	TextBody  string
	HTMLBody  string
	HasHTML   bool
	RawHeader mail.Header
}

// ParseMailData parses SMTP DATA (RFC822) and extracts subject/text/html bodies.
func ParseMailData(raw []byte, allowHTML bool) (ParsedMail, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return ParsedMail{}, fmt.Errorf("parse RFC822 message: %w", err)
	}
	res := ParsedMail{RawHeader: msg.Header}
	res.Subject = decodeHeader(msg.Header.Get("Subject"))
	if res.Subject == "" {
		res.Subject = "(no subject)"
	}

	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil || mediaType == "" {
		body, readErr := io.ReadAll(msg.Body)
		if readErr != nil {
			return ParsedMail{}, fmt.Errorf("read message body: %w", readErr)
		}
		res.TextBody = strings.TrimSpace(string(body))
		return res, nil
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return ParsedMail{}, fmt.Errorf("multipart body missing boundary")
		}
		mr := multipart.NewReader(msg.Body, boundary)
		for {
			p, e := mr.NextPart()
			if e == io.EOF {
				break
			}
			if e != nil {
				return ParsedMail{}, fmt.Errorf("read multipart body: %w", e)
			}
			ct, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
			partBody, _ := io.ReadAll(p)
			text := strings.TrimSpace(string(partBody))
			switch ct {
			case "text/plain":
				if res.TextBody == "" {
					res.TextBody = text
				}
			case "text/html":
				if allowHTML && res.HTMLBody == "" {
					res.HTMLBody = text
					res.HasHTML = text != ""
				}
			}
		}
		if res.TextBody == "" && !res.HasHTML {
			res.TextBody = "(empty body)"
		}
		return res, nil
	}

	body, readErr := io.ReadAll(msg.Body)
	if readErr != nil {
		return ParsedMail{}, fmt.Errorf("read message body: %w", readErr)
	}
	content := strings.TrimSpace(string(body))
	if mediaType == "text/html" && allowHTML {
		res.HTMLBody = content
		res.HasHTML = content != ""
		return res, nil
	}
	res.TextBody = content
	return res, nil
}

func decodeHeader(v string) string {
	if v == "" {
		return ""
	}
	d := new(mime.WordDecoder)
	decoded, err := d.DecodeHeader(v)
	if err != nil {
		return v
	}
	return decoded
}
