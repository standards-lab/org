package person

import (
	"errors"
	"fmt"
	"regexp"
	"time"
	"uuid"
)

var (
	// ErrValidation classifies a command input rejection; wrapped with the
	// field-level reason, it reaches the wire as a 400's detail.
	ErrValidation = errors.New("invalid command")

	// ErrTransition reports an action the record's current status does not
	// allow: activating an active person, deactivating one not active.
	ErrTransition = errors.New("transition not allowed")
)

// emailPattern mirrors the schema's cc_person_email check.
var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+$`)

// Status is the record status other domains react to.
type Status string

const (
	StatusPending  Status = "pending"
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
)

// Person is one record, as the read contract presents it. Version is the
// concurrency token the commands and actions guard on.
type Person struct {
	ID         string    `json:"id"`
	UnitID     string    `json:"unit_id"`
	GivenName  string    `json:"given_name"`
	FamilyName string    `json:"family_name"`
	Email      string    `json:"email"`
	Status     Status    `json:"status"`
	Version    int64     `json:"version"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// CreatePerson is the create command's input; the record starts pending.
type CreatePerson struct {
	UnitID     string `json:"unit_id"`
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
	Email      string `json:"email"`
}

// Validate rejects what the command can know from its fields; the unit's
// existence and the email's uniqueness are the store's.
func (c CreatePerson) Validate() error {
	return errors.Join(validUnit(c.UnitID), validName("given_name", c.GivenName), validName("family_name", c.FamilyName), validEmail(c.Email))
}

// EditPerson is the edit command's input: full replacement of the
// client-mutable descriptive fields.
type EditPerson struct {
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
	Email      string `json:"email"`
}

// Validate rejects an empty name or a malformed email.
func (e EditPerson) Validate() error {
	return errors.Join(validName("given_name", e.GivenName), validName("family_name", e.FamilyName), validEmail(e.Email))
}

// TransferUnit is the transfer-unit action's input.
type TransferUnit struct {
	UnitID string `json:"unit_id"`
}

// Validate rejects a unit id that is not a UUID.
func (t TransferUnit) Validate() error { return validUnit(t.UnitID) }

// Identity is every command's success envelope.
type Identity struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}

// state is an action's precondition read: the record as the transition
// rule sees it.
type state struct {
	Status  Status `json:"status"`
	Version int64  `json:"version"`
}

// The transition rules, one per action, over the record's current status.

func (s state) canActivate() error {
	if s.Status == StatusActive {
		return fmt.Errorf("%w: already active", ErrTransition)
	}
	return nil
}

func (s state) canDeactivate() error {
	if s.Status != StatusActive {
		return fmt.Errorf("%w: only an active person deactivates, status is %s", ErrTransition, s.Status)
	}
	return nil
}

// The field rules the commands share.

func validUnit(id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("%w: unit_id must be a UUID", ErrValidation)
	}
	return nil
}

func validName(field, v string) error {
	if v == "" {
		return fmt.Errorf("%w: %s must not be empty", ErrValidation, field)
	}
	return nil
}

func validEmail(email string) error {
	if !emailPattern.MatchString(email) {
		return fmt.Errorf("%w: email must be an address", ErrValidation)
	}
	return nil
}
