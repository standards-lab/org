package volume

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidAddress reports a command-line address that does not have the
// form <volume>:<path>.
var ErrInvalidAddress = errors.New("volume: invalid address")

// Address is a place in the file system as the command line names it: a
// volume's name and a path inside that volume, written <volume>:<path>.
// docs:/ is the root of the volume docs and docs:/reports/2026 a directory
// two levels below it. The path's segments are validated when the address
// is resolved, by the library's path rules.
type Address struct {
	Volume string
	Path   string
}

// ParseAddress reads s as <volume>:<path>. The colon is required, the
// volume name must not be empty, and the path must start with a slash. A
// refusal is ErrInvalidAddress with the reason.
func ParseAddress(s string) (Address, error) {
	name, path, ok := strings.Cut(s, ":")
	switch {
	case !ok:
		return Address{}, fmt.Errorf("%w: %q has no colon; write <volume>:<path>, for example docs:/reports", ErrInvalidAddress, s)
	case name == "":
		return Address{}, fmt.Errorf("%w: %q names no volume before the colon", ErrInvalidAddress, s)
	case !strings.HasPrefix(path, "/"):
		return Address{}, fmt.Errorf("%w: the path %q does not start with /", ErrInvalidAddress, path)
	}
	return Address{Volume: name, Path: path}, nil
}

// String returns the address as the command line writes it.
func (a Address) String() string {
	return a.Volume + ":" + a.Path
}

// Split returns the address of the directory that contains a's last
// segment and that segment, for the commands that create the last segment
// under an existing parent. The root of a volume has no parent and is
// refused, as is a path that ends with a slash, since its last segment is
// empty.
func (a Address) Split() (Address, string, error) {
	if a.Path == "/" {
		return Address{}, "", fmt.Errorf("%w: %s is the volume's root, which has no parent", ErrInvalidAddress, a)
	}
	i := strings.LastIndex(a.Path, "/")
	parent, name := a.Path[:i], a.Path[i+1:]
	if name == "" {
		return Address{}, "", fmt.Errorf("%w: %s ends with a slash and names no last segment", ErrInvalidAddress, a)
	}
	if parent == "" {
		parent = "/"
	}
	return Address{Volume: a.Volume, Path: parent}, name, nil
}

// Join returns the address of name under a: the path gains a slash and the
// name, except under the root, which already ends with its slash.
func (a Address) Join(name string) Address {
	if a.Path == "/" {
		return Address{Volume: a.Volume, Path: "/" + name}
	}
	return Address{Volume: a.Volume, Path: a.Path + "/" + name}
}
