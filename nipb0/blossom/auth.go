package blossom

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"time"

	"github.com/nosuta/nostr"
)

// KindAuth is the Blossom authorization event kind (BUD-11). It is the same
// kind as nostr.KindBlobs; the alias spells out the BUD-11 role at call sites.
const KindAuth = nostr.KindBlobs

// DefaultAuthTTL bounds an authorization token's life. Blossom requires an
// `expiration` tag; a short window limits what a leaked token can do.
const DefaultAuthTTL = 5 * time.Minute

// AuthorizationEvent builds the unsigned BUD-11 authorization event for verb
// (`get`, `upload`, `delete`, `list`, `media`) over hashes. A non-empty server
// adds a `server` tag binding the token to that host. A non-positive ttl uses
// DefaultAuthTTL; an empty content uses "Blossom <verb>".
//
// It performs no I/O so the same code works with net/http on native and the
// browser fetch API on Web. The caller signs the event and can pass it to
// AuthorizationHeaderValue.
func AuthorizationEvent(verb string, hashes []string, server, content string, ttl time.Duration) nostr.Event {
	if ttl <= 0 {
		ttl = DefaultAuthTTL
	}
	if content == "" {
		content = "Blossom " + verb
	}
	now := time.Now()
	evt := nostr.Event{
		CreatedAt: nostr.Timestamp(now.Unix()),
		Kind:      KindAuth,
		Content:   content,
		Tags: nostr.Tags{
			nostr.Tag{"t", verb},
			nostr.Tag{"expiration", strconv.FormatInt(now.Add(ttl).Unix(), 10)},
		},
	}
	for _, h := range hashes {
		if h != "" {
			evt.Tags = append(evt.Tags, nostr.Tag{"x", h})
		}
	}
	if server != "" {
		evt.Tags = append(evt.Tags, nostr.Tag{"server", server})
	}
	return evt
}

// AuthorizationHeader signs a BUD-11 authorization event with signer and
// returns the `Authorization` header value: "Nostr <base64(event JSON)>",
// ready to send verbatim. See AuthorizationEvent for the tag semantics.
func AuthorizationHeader(ctx context.Context, signer nostr.Signer, verb string, hashes []string, server, content string, ttl time.Duration) (string, error) {
	evt := AuthorizationEvent(verb, hashes, server, content, ttl)
	if err := signer.SignEvent(ctx, &evt); err != nil {
		return "", err
	}
	return AuthorizationHeaderValue(evt)
}

// AuthorizationHeaderValue encodes an already-signed BUD-11 event as an
// `Authorization` header value.
func AuthorizationHeaderValue(evt nostr.Event) (string, error) {
	raw, err := json.Marshal(evt)
	if err != nil {
		return "", err
	}
	return "Nostr " + base64.StdEncoding.EncodeToString(raw), nil
}
