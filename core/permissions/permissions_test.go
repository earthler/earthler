package permissions

import "testing"

// contract is the table again, written out by hand. A change of who may do
// what has to change it here too, on purpose.
var contract = map[Action]map[Role]bool{
	WriteInOpenChannel: {Child: false, Resident: true, Moderator: true, Administrator: true},
	RemoveContent:      {Child: false, Resident: false, Moderator: true, Administrator: true},
	ChangeRoles:        {Child: false, Resident: false, Moderator: false, Administrator: true},
	SeeResidents:       {Child: false, Resident: false, Moderator: false, Administrator: true},
	ReadTheLog:         {Child: true, Resident: true, Moderator: true, Administrator: true},
}

func TestTheTableSaysWhatWeAgreed(t *testing.T) {
	for action, byRole := range contract {
		for _, role := range Roles {
			if got := Can(role, action); got != byRole[role] {
				t.Errorf("Can(%s, %s) = %v, want %v", role, action, got, byRole[role])
			}
		}
	}
	if len(Actions()) != len(contract) {
		t.Errorf("the table has %d actions and the contract %d: every action needs its line in both", len(Actions()), len(contract))
	}
}

func TestWhatIsNotListedIsNotAllowed(t *testing.T) {
	for _, role := range []Role{"", "root", "Administrator", "admin", " moderator"} {
		for _, action := range Actions() {
			if Can(role, action) {
				t.Errorf("a role called %q may %s", role, action)
			}
		}
	}
	for _, action := range []Action{"", "chat.read", "everything", "chat.write "} {
		for _, role := range Roles {
			if Can(role, action) {
				t.Errorf("%s may do an action called %q", role, action)
			}
		}
	}
}

func TestOnlyRealRolesCanBeGivenByHand(t *testing.T) {
	for role, want := range map[Role]bool{Resident: true, Moderator: true, Administrator: true, Child: false, "": false, "root": false} {
		if got := Assignable(role); got != want {
			t.Errorf("Assignable(%q) = %v, want %v", role, got, want)
		}
	}
	for _, role := range Roles {
		if !Valid(role) {
			t.Errorf("%s is listed and not valid", role)
		}
	}
}

func TestPowersGrowWithTheRole(t *testing.T) {
	// Whoever has a power also has it one role up, so a promotion never
	// takes something away.
	for _, action := range Actions() {
		had := false
		for _, role := range Roles {
			can := Can(role, action)
			if had && !can {
				t.Errorf("%s may %s but %s, one role up, may not", Roles, action, role)
			}
			had = had || can
		}
	}
}
