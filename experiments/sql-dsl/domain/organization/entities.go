package organization

import (
	"errors"
	"fmt"
	"regexp"
	"time"
	"uuid"
)

// ErrValidation classifies a command input rejection; wrapped with the
// field-level reason, it reaches the wire as a 400's detail.
var ErrValidation = errors.New("invalid command")

// codePattern mirrors the schema's cc_organization_code check, so a bad
// code is a 400 with detail rather than a database check violation.
var codePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Organization is one node of the organization hierarchy, as the read
// contract presents it. ParentID is nil at a root. Path is composed at read
// time from the lineage and never stored. Version is the concurrency token
// the commands guard on.
type Organization struct {
	ID        string    `json:"id"`
	ParentID  *string   `json:"parent_id"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Path      string    `json:"path"`
}

// CreateOrganization is the create command's input: the parent under which
// the organization is created — nil for a root — and its code and name.
type CreateOrganization struct {
	ParentID *string `json:"parent_id"`
	Code     string  `json:"code"`
	Name     string  `json:"name"`
}

// Validate rejects what the command can know from its own fields; the
// parent's existence and the sibling code's uniqueness are the store's.
func (c CreateOrganization) Validate() error {
	return errors.Join(validCode(c.Code), validName(c.Name), validParent(c.ParentID))
}

// EditOrganization is the edit command's input: full replacement of the two
// client-mutable descriptive fields.
type EditOrganization struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Validate rejects a malformed code or an empty name.
func (e EditOrganization) Validate() error {
	return errors.Join(validCode(e.Code), validName(e.Name))
}

// TransferOrganization is the transfer command's input. A nil ParentID —
// stated as null or omitted — moves the organization to the root.
type TransferOrganization struct {
	ParentID *string `json:"parent_id"`
}

// Validate rejects a parent id that is not a UUID; nil is the root.
func (t TransferOrganization) Validate() error { return validParent(t.ParentID) }

// Identity is every command's success envelope: the row's id and the
// version the command left it at. Commands return nothing else.
type Identity struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}

// The field rules the commands share.

func validCode(code string) error {
	if !codePattern.MatchString(code) {
		return fmt.Errorf("%w: code must be lowercase words joined by single hyphens", ErrValidation)
	}
	return nil
}

func validName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name must not be empty", ErrValidation)
	}
	return nil
}

func validParent(parent *string) error {
	if parent == nil {
		return nil
	}
	if _, err := uuid.Parse(*parent); err != nil {
		return fmt.Errorf("%w: parent_id must be a UUID", ErrValidation)
	}
	return nil
}
