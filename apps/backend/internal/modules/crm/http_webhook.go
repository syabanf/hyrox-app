package crm

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"

	"github.com/syabanf/hyrox-app/apps/backend/internal/platform/httpx"
)

// The Instagram webhook.
//
// Two things make an inbound webhook different from every other endpoint here,
// and both are about not trusting the caller:
//
//   - It is authenticated by a signature over the raw body, not by a session.
//     Anybody can POST to a public URL, so the question is not who is logged in
//     but whether this body was written by somebody holding the secret.
//   - It is redelivered. Meta retries on any non-2xx and sometimes on a 2xx it
//     did not hear, so the same message arrives more than once and must land
//     once. The provider's message id is the idempotency key, and the unique
//     index on it is what actually enforces this.
//
// It answers 200 to almost everything on purpose. A 500 makes Meta retry, and
// a message we cannot parse will not parse the second time either — it goes in
// the log, not into a retry loop.

// InstagramWebhook holds what the verification needs.
type InstagramWebhook struct {
	service *Service
	// VerifyToken is echoed back during the one-time subscription handshake.
	verifyToken string
	// AppSecret signs every delivery. Empty means unconfigured, and an
	// unconfigured webhook refuses everything rather than accepting anything.
	appSecret string
}

func NewInstagramWebhook(service *Service, verifyToken, appSecret string) *InstagramWebhook {
	return &InstagramWebhook{service: service, verifyToken: verifyToken, appSecret: appSecret}
}

func (h *InstagramWebhook) Mount(r *httpx.Router) {
	// Both are public: the caller is Meta, which has no session. The GET is
	// the subscription handshake and the POST carries the messages.
	r.Get("/api/webhooks/instagram", h.verify)
	r.Post("/api/webhooks/instagram", h.receive)
}

// verify answers the subscription handshake.
//
// Meta calls this once when the webhook is registered, with a challenge and
// the token we gave it. Echoing the challenge back proves we own the endpoint.
func (h *InstagramWebhook) verify(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("hub.mode")
	token := r.URL.Query().Get("hub.verify_token")
	challenge := r.URL.Query().Get("hub.challenge")

	if h.verifyToken == "" || mode != "subscribe" || token != h.verifyToken {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(challenge))
}

// instagramDelivery is the shape Meta posts. Only the parts that carry a
// message are modelled: the rest changes without notice and ignoring it is
// cheaper than tracking it.
type instagramDelivery struct {
	Object string `json:"object"`
	Entry  []struct {
		ID        string `json:"id"`
		Messaging []struct {
			Sender    struct{ ID string } `json:"sender"`
			Timestamp int64               `json:"timestamp"`
			Message   struct {
				MID  string `json:"mid"`
				Text string `json:"text"`
			} `json:"message"`
		} `json:"messaging"`
	} `json:"entry"`
}

func (h *InstagramWebhook) receive(w http.ResponseWriter, r *http.Request) {
	// An unconfigured webhook accepts nothing. A publicly reachable endpoint
	// that writes to the inbox without checking a signature is an open door.
	if h.appSecret == "" {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("This webhook is not configured."))
		return
	}

	// The signature covers the raw bytes, so the body has to be read before it
	// is parsed — decoding first and re-encoding would sign something subtly
	// different from what arrived.
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		httpx.Fail(w, r, httpx.Invalid("That delivery could not be read."))
		return
	}
	if !h.signed(body, r.Header.Get("X-Hub-Signature-256")) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("That delivery is not signed by the app."))
		return
	}

	delivery, err := httpx.DecodeBytes[instagramDelivery](body)
	if err != nil {
		// Deliberately 200: a body we cannot parse will not parse on the
		// retry either, and answering anything else buys an infinite loop.
		httpx.OK(w, map[string]string{"status": "ignored"})
		return
	}

	accepted := 0
	for _, entry := range delivery.Entry {
		for _, event := range entry.Messaging {
			text := strings.TrimSpace(event.Message.Text)
			if text == "" || event.Message.MID == "" {
				continue
			}
			handle := event.Sender.ID
			mid := event.Message.MID
			// Straight into the ordinary inbox. An Instagram DM is a message
			// from a person, and giving it its own parallel world is how a
			// support desk ends up with two of everything.
			if _, err := h.service.OpenConversation(r.Context(), ConversationInput{
				ContactName: "Instagram " + handle, ContactHandle: &handle,
				Channel: "INSTAGRAM", Subject: "Instagram message",
				Body: text, ExternalID: &mid, Inbound: true,
			}, Actor{ID: "webhook:instagram", Name: "Instagram"}); err != nil {
				httpx.Fail(w, r, err)
				return
			}
			accepted++
		}
	}
	httpx.OK(w, map[string]int{"accepted": accepted})
}

// signed checks Meta's HMAC over the raw body.
//
// hmac.Equal rather than == : a byte-by-byte comparison that returns early
// leaks, through timing, how much of a forged signature was right.
func (h *InstagramWebhook) signed(body []byte, header string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(h.appSecret))
	mac.Write(body)
	return hmac.Equal(provided, mac.Sum(nil))
}
