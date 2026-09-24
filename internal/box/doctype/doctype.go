// Package doctype is the catalogue of what a scan in a Box can be: the type
// names that go into a sidecar, how long each stays useful, and the key that
// chooses it during intake. It holds no files and draws nothing, so the intake
// page, the browser and a report all offer the same list.
//
// The catalogue is registered here rather than in configuration. A type name is
// written into a sidecar and has to mean the same thing on another machine, and
// a lifetime is part of what a type is. The catalogue is documented in
// docs/apps/box/types.md, and a test in this package fails when that document
// stops naming every type registered here.
package doctype

import "sort"

// Nature is why a scan is kept, which is what decides whether it can stop being
// useful. A Utility scan is kept because it may be needed and has an end; a
// Keepsake is kept because its owner wants it and has none. Half a Box is
// Keepsake, which is why asking every scan for an expiry and an amount would
// only make intake slower.
type Nature int

const (
	Utility Nature = iota
	Keepsake
)

func (n Nature) String() string {
	if n == Keepsake {
		return "keepsake"
	}
	return "utility"
}

// Type is one entry of the catalogue.
type Type struct {
	// Name is what goes into a sidecar: a lowercase ASCII slug, stable forever,
	// because renaming one means rewriting every sidecar that used it.
	Name string
	// Label is what a person reads on screen.
	Label string
	// Covers says what belongs under this type, in the words of
	// docs/apps/box/types.md; the intake page shows it on hover.
	Covers string
	// Nature is why this kind of thing is kept.
	Nature Nature
	// Lifetime is how many days after the event date a scan of this type stops
	// being useful. Nil means it never does and no arithmetic is done, which is
	// not the same as Days(0) — a ticket stub expires on the day of the event.
	Lifetime *int
	// ExpiryExpected marks a type whose expiry is printed on the document and is
	// the point of recording it, such as a passport. There is no sane default to
	// guess, so there is none; view lists the scans still missing a date instead.
	ExpiryExpected bool
	// Key is the single keystroke that chooses this type during intake. Every
	// Key in the catalogue is distinct, which a test checks: a type is chosen a
	// few hundred times in a row, so the list has to be operable without the
	// hand leaving the keyboard.
	Key rune
}

// Permanent reports whether this type gives its scans no expiry of their own. A
// scan of a permanent type can still be given one by hand.
func (t Type) Permanent() bool { return t.Lifetime == nil }

// Days is a lifetime in days, for the catalogue below. Days(0) is the event
// date itself.
func Days(count int) *int { return &count }

const year = 365

// Unsorted is the type a scan is taken in with when nobody can say what it is
// yet. It is a state rather than a kind of document, and it is what makes an
// inbox drainable: intake never blocks on a decision, because this is always an
// available answer.
const Unsorted = "unsorted"

// catalogue is every registered type, in the order the intake page offers them:
// utility first, then keepsake, then the two that describe an absence.
var catalogue = []Type{
	{Name: "ticket", Label: "Ticket stub", Covers: "Ticket stubs: cinema, concerts, local transport", Nature: Utility, Lifetime: Days(0), Key: 't'},
	{Name: "travel", Label: "Travel", Covers: "Flights, boarding passes, hotel confirmations, car hire", Nature: Utility, Lifetime: Days(90), Key: 'a'},
	{Name: "receipt", Label: "Receipt", Covers: "Receipts and invoices", Nature: Utility, Lifetime: Days(7 * year), Key: 'r'},
	{Name: "statement", Label: "Statement", Covers: "Bills and account statements", Nature: Utility, Lifetime: Days(7 * year), Key: 's'},
	{Name: "insurance", Label: "Insurance", Covers: "Policies, certificates of cover", Nature: Utility, Lifetime: Days(year), ExpiryExpected: true, Key: 'i'},
	{Name: "invite", Label: "Invitation", Covers: "Invitations", Nature: Utility, Lifetime: Days(0), Key: 'n'},
	{Name: "contract", Label: "Contract", Covers: "Contracts and agreements", Nature: Utility, Key: 'c'},
	{Name: "identity", Label: "Identity document", Covers: "Identity documents, visas, licences", Nature: Utility, ExpiryExpected: true, Key: 'd'},
	{Name: "medical", Label: "Medical", Covers: "Prescriptions, results, medical receipts", Nature: Utility, Key: 'm'},
	{Name: "manual", Label: "Manual", Covers: "Instruction manuals and user guides for things owned", Nature: Utility, Key: 'h'},
	{Name: "letter", Label: "Letter", Covers: "Correspondence worth keeping", Nature: Keepsake, Key: 'l'},
	{Name: "ephemera", Label: "Ephemera", Covers: "Printed matter kept for its own sake: leaflets, brochures, an advertisement, a programme", Nature: Keepsake, Key: 'e'},
	{Name: "object", Label: "Object", Covers: "A scan of a small object rather than of a document", Nature: Keepsake, Key: 'o'},
	{Name: "other", Label: "Other", Covers: "Anything real that no type above fits", Nature: Keepsake, Key: 'z'},
	{Name: Unsorted, Label: "Unsorted", Covers: "Not yet decided", Nature: Keepsake, Key: 'u'},
}

// All is the catalogue in the order intake offers it.
func All() []Type { return append([]Type(nil), catalogue...) }

var byName = func() map[string]Type {
	index := make(map[string]Type, len(catalogue))
	for _, entry := range catalogue {
		index[entry.Name] = entry
	}
	return index
}()

// Lookup finds a type by the name in a sidecar. A name this build does not
// register is not an error for a reader: the sidecar keeps it, the screen shows
// it as unknown, and nothing rewrites it to "other". Only writing validates.
func Lookup(name string) (Type, bool) {
	entry, known := byName[name]
	return entry, known
}

// Names is every registered name, sorted, for an error message that has to say
// what was expected.
func Names() []string {
	names := make([]string, 0, len(catalogue))
	for _, entry := range catalogue {
		names = append(names, entry.Name)
	}
	sort.Strings(names)
	return names
}
