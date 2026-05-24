// Package server — handle_pairing.go
//
// HTTP handlers for the device-pairing-by-short-code flow. See
// pkg/auth/pairing.go for the state machine and security argument.
//
// Endpoints:
//   POST /auth/devices/pair/begin    — anonymous. New device starts pairing.
//   GET  /auth/devices/pair/status   — anonymous. New device polls.
//   POST /auth/devices/pair/complete — anonymous. New device finalizes the
//                                       WebAuthn ceremony after approval.
//   GET  /me/devices/pair/lookup     — authed. Show pairing context (UA/IP)
//                                       before user confirms approval.
//   POST /me/devices/pair/approve    — authed. Bind pairing to caller's
//                                       account.
//   POST /me/devices/pair/cancel     — authed. Drop a pending pairing.
package server

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/sirupsen/logrus"

	"picotera/pkg/auth"
	"picotera/pkg/contract"
	"picotera/pkg/db"
	"picotera/pkg/logx"
)

// ----- POST /auth/devices/pair/begin (anonymous) ---------------------------

type pairBeginResp struct {
	PairingID     string                            `json:"pairingId"`
	Code          string                            `json:"code"`           // raw 8-char
	DisplayCode   string                            `json:"displayCode"`    // "XXXX-XXXX"
	ExpiresAt     time.Time                         `json:"expiresAt"`
	PublicKey     *protocol.PublicKeyCredentialCreationOptions `json:"publicKey"` // for credentials.create() once approved
}

func (s *Server) handlePairBeginHTTP(w http.ResponseWriter, r *http.Request) {
	// Build a "blank" WebAuthn user for the registration ceremony. The actual
	// account isn't known until the approver binds it, so we generate a
	// throwaway user handle. After approval we substitute the real account's
	// user handle into the SessionData before calling CreateCredential so
	// go-webauthn's verification chooses the right RP.
	throwaway := make([]byte, 64)
	if _, err := rand.Read(throwaway); err != nil {
		writeAuthErr(w, fmt.Errorf("pair/begin: random: %w", err))
		return
	}
	wu := &auth.WebAuthnAccount{Account: &db.Account{
		Username:           "__pairing__",
		DisplayName:        "Pairing in progress",
		WebauthnUserHandle: throwaway,
	}}

	creation, sessionData, err := s.webauthn.BeginRegistration(wu, auth.RegistrationOptions(nil)...)
	if err != nil {
		writeAuthErr(w, auth.MapRegisterCeremonyError(r.Context(), err))
		return
	}

	ua := r.UserAgent()
	ip := auth.ClientIP(r, s.config.TrustProxy)
	p, err := s.pairingStore.New(r.Context(), *sessionData, ua, ip)
	if err != nil {
		writeAuthErr(w, fmt.Errorf("pair/begin: store: %w", err))
		return
	}

	logx.WithContext(r.Context()).WithFields(logrus.Fields{
		"event":      "auth.pairing_begin",
		"pairing_id": p.ID,
		"client_ip":  ip,
	}).Info("auth")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pairBeginResp{
		PairingID:   p.ID,
		Code:        p.Code,
		DisplayCode: auth.FormatPairingCode(p.Code),
		ExpiresAt:   p.ExpiresAt,
		PublicKey:   &creation.Response,
	})
}

// ----- GET /auth/devices/pair/status (anonymous, polled) -------------------

type pairStatusResp struct {
	Status    string    `json:"status"` // pending | approved | consumed | expired | not_found
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

func (s *Server) handlePairStatusHTTP(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeAuthErr(w, auth.ErrPairingNotFound())
		return
	}
	p, err := s.pairingStore.GetByID(r.Context(), id)
	if err != nil {
		// Distinguish "not found" (404) from real errors. The PC polling
		// this endpoint expects to receive a status string even on expiry
		// so it can show "expired, please retry" in the UI; treat
		// not-found as expired here since the KV TTL handles both.
		if ae := auth.AsAuthError(err); ae != nil && ae.Code == "pairing_not_found" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(pairStatusResp{Status: "expired"})
			return
		}
		writeAuthErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pairStatusResp{
		Status:    string(p.Status),
		ExpiresAt: p.ExpiresAt,
	})
}

// ----- POST /auth/devices/pair/complete (anonymous, bound by pairing) -----

type pairCompleteReq struct {
	PairingID   string          `json:"pairingId"`
	Attestation json.RawMessage `json:"attestation"`
	Nickname    string          `json:"nickname,omitempty"`
}

type pairCompleteResp struct {
	Session         contract.SessionView `json:"session"`
	NewCredentialID int32                `json:"newCredentialId"`
}

func (s *Server) handlePairCompleteHTTP(w http.ResponseWriter, r *http.Request) {
	var body pairCompleteReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAuthErr(w, auth.ErrCeremonyState())
		return
	}
	if body.PairingID == "" {
		writeAuthErr(w, auth.ErrPairingNotFound())
		return
	}

	p, err := s.pairingStore.GetByID(r.Context(), body.PairingID)
	if err != nil {
		writeAuthErr(w, err)
		return
	}
	if p.Status != auth.PairingApproved {
		writeAuthErr(w, auth.ErrPairingNotApproved())
		return
	}

	acct, err := s.queries.GetAccountByID(r.Context(), p.ApprovedFor)
	if err != nil {
		writeAuthErr(w, auth.ErrAccountNotFound())
		return
	}
	if acct.Disabled {
		writeAuthErr(w, auth.ErrAccountDisabled())
		return
	}

	// Substitute the real account's user handle into the session data so
	// CreateCredential's user-id check passes. The pair/begin used a
	// throwaway handle because the account wasn't yet known.
	sessionData := p.SessionData
	sessionData.UserID = acct.WebauthnUserHandle

	parsed, err := protocol.ParseCredentialCreationResponseBytes(body.Attestation)
	if err != nil {
		writeAuthErr(w, auth.MapRegisterCeremonyError(r.Context(), err))
		return
	}
	existing, _ := s.queries.ListCredentialsByAccount(r.Context(), acct.ID)
	wu := &auth.WebAuthnAccount{Account: &acct, Credentials: existing}
	cred, err := s.webauthn.CreateCredential(wu, sessionData, parsed)
	if err != nil {
		writeAuthErr(w, auth.MapRegisterCeremonyError(r.Context(), err))
		return
	}

	var validatedNickname *string
	if body.Nickname != "" {
		if err := auth.ValidateNickname(&body.Nickname); err != nil {
			writeAuthErr(w, err)
			return
		}
		validatedNickname = auth.NormalizeNickname(&body.Nickname)
	}

	transports := make([]string, 0, len(cred.Transport))
	for _, t := range cred.Transport {
		transports = append(transports, string(t))
	}
	row, err := s.queries.InsertCredential(r.Context(), db.InsertCredentialParams{
		AccountID:       acct.ID,
		CredentialID:    cred.ID,
		PublicKey:       cred.PublicKey,
		SignCount:       int64(cred.Authenticator.SignCount),
		Transports:      transports,
		Aaguid:          cred.Authenticator.AAGUID,
		AttestationType: cred.AttestationType,
		BackupEligible:  cred.Flags.BackupEligible,
		BackupState:     cred.Flags.BackupState,
		Nickname:        nicknameParamPtr(validatedNickname),
	})
	if err != nil {
		writeAuthErr(w, fmt.Errorf("pair/complete: insert credential: %w", err))
		return
	}

	if err := s.pairingStore.Consume(r.Context(), p); err != nil {
		writeAuthErr(w, err)
		return
	}

	ip := auth.ClientIP(r, s.config.TrustProxy)
	sessionToken, _, err := s.sessionStore.Issue(r.Context(), acct.ID, ip)
	if err != nil {
		writeAuthErr(w, fmt.Errorf("pair/complete: session issue: %w", err))
		return
	}
	http.SetCookie(w, auth.FreshSessionCookie(s.config, r, acct.ID, sessionToken, s.config.SessionTTL))

	logx.WithContext(r.Context()).WithFields(logrus.Fields{
		"event":         "auth.pairing_completed",
		"account_id":    acct.ID,
		"pairing_id":    p.ID,
		"credential_id": row.ID,
		"client_ip":     ip,
	}).Info("auth")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pairCompleteResp{
		Session:         sessionView(&acct),
		NewCredentialID: row.ID,
	})
}

// ----- GET /me/devices/pair/lookup (authed) --------------------------------
//
// User types the code on /me; the lookup returns the pairing's initiator
// context (UA + IP + age) so the user can verify it's really their other
// device before approving. Lookup is read-only — does NOT bind the pairing.

type pairLookupResp struct {
	PairingID     string    `json:"pairingId"`
	InitiatorUA   string    `json:"initiatorUa"`
	InitiatorIP   string    `json:"initiatorIp"`
	CreatedAt     time.Time `json:"createdAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
	AlreadyBound  bool      `json:"alreadyBound"`
}

func (s *Server) handlePairLookupHTTP(w http.ResponseWriter, r *http.Request) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil {
		writeAuthErr(w, auth.ErrNoSession())
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		writeAuthErr(w, auth.ErrPairingNotFound())
		return
	}
	p, err := s.pairingStore.LookupByCode(r.Context(), code)
	if err != nil {
		writeAuthErr(w, err)
		return
	}
	// If already approved for someone else, refuse to surface it (treat as
	// not_found — the code is no longer claimable by this caller).
	if p.Status == auth.PairingApproved && p.ApprovedFor != sess.Account.ID {
		writeAuthErr(w, auth.ErrPairingNotFound())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pairLookupResp{
		PairingID:    p.ID,
		InitiatorUA:  p.InitiatorUA,
		InitiatorIP:  p.InitiatorIP,
		CreatedAt:    p.CreatedAt,
		ExpiresAt:    p.ExpiresAt,
		AlreadyBound: p.Status == auth.PairingApproved && p.ApprovedFor == sess.Account.ID,
	})
}

// ----- POST /me/devices/pair/approve (authed) ------------------------------

type pairApproveReq struct {
	Code string `json:"code"`
}

func (s *Server) handlePairApproveHTTP(w http.ResponseWriter, r *http.Request) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil {
		writeAuthErr(w, auth.ErrNoSession())
		return
	}
	var body pairApproveReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAuthErr(w, auth.ErrPairingNotFound())
		return
	}
	p, err := s.pairingStore.LookupByCode(r.Context(), body.Code)
	if err != nil {
		writeAuthErr(w, err)
		return
	}
	if err := s.pairingStore.Approve(r.Context(), p, sess.Account.ID); err != nil {
		writeAuthErr(w, err)
		return
	}
	logx.WithContext(r.Context()).WithFields(logrus.Fields{
		"event":      "auth.pairing_approved",
		"pairing_id": p.ID,
		"account_id": sess.Account.ID,
		"client_ip":  auth.ClientIP(r, s.config.TrustProxy),
	}).Info("auth")
	w.WriteHeader(http.StatusNoContent)
}

// ----- POST /me/devices/pair/cancel (authed) -------------------------------

type pairCancelReq struct {
	Code string `json:"code"`
}

func (s *Server) handlePairCancelHTTP(w http.ResponseWriter, r *http.Request) {
	sess := auth.SessionFromContext(r.Context())
	if sess == nil {
		writeAuthErr(w, auth.ErrNoSession())
		return
	}
	var body pairCancelReq
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAuthErr(w, auth.ErrPairingNotFound())
		return
	}
	p, err := s.pairingStore.LookupByCode(r.Context(), body.Code)
	if err != nil {
		writeAuthErr(w, err)
		return
	}
	// Refuse to cancel a pairing approved by a different account — the
	// approver owns it once approved.
	if p.Status == auth.PairingApproved && p.ApprovedFor != sess.Account.ID {
		writeAuthErr(w, auth.ErrPairingNotFound())
		return
	}
	if err := s.pairingStore.Cancel(r.Context(), p); err != nil {
		writeAuthErr(w, err)
		return
	}
	logx.WithContext(r.Context()).WithFields(logrus.Fields{
		"event":      "auth.pairing_cancelled",
		"pairing_id": p.ID,
		"account_id": sess.Account.ID,
	}).Info("auth")
	w.WriteHeader(http.StatusNoContent)
}

