// Package permissions says who may do what. The whole table is in this one
// file, and no screen decides on its own: a screen asks Can, and a change of
// the rules is a change of this table and of its test.
package permissions

// Role is what a resident is allowed to be in the community.
type Role string

const (
	// Resident is everyone who lives here and has no other role.
	Resident Role = "resident"
	// Child reads what the people responsible for them allow, and does not
	// write in the open channels. Making someone a child needs a guardian,
	// which is not built yet, so nobody is given this role by hand.
	Child Role = "child"
	// Moderator removes content from the open channels, with a reason that
	// stays in the log.
	Moderator Role = "moderator"
	// Administrator looks after the community: roles, and what is done with
	// the machine as it comes.
	Administrator Role = "administrator"
)

// Roles lists every role, from the one with the fewest powers up.
var Roles = []Role{Child, Resident, Moderator, Administrator}

// Valid says whether r is a role that exists.
func Valid(r Role) bool {
	for _, known := range Roles {
		if r == known {
			return true
		}
	}
	return false
}

// Assignable says whether an administrator may give r to someone by hand.
// A child is made through a guardian, not through a menu.
func Assignable(r Role) bool { return Valid(r) && r != Child }

// Action is one thing a resident can try to do.
type Action string

const (
	// WriteInOpenChannel is writing a message in an open channel.
	WriteInOpenChannel Action = "chat.write"
	// RemoveContent is removing what someone else wrote, with a reason.
	RemoveContent Action = "content.remove"
	// ChangeRoles is giving someone another role.
	ChangeRoles Action = "roles.change"
	// SeeResidents is the list of residents with their roles.
	SeeResidents Action = "residents.see"
	// ReadTheLog is reading what the people with powers did with them. The
	// log is open to every resident on purpose.
	ReadTheLog Action = "log.read"
)

// table says who may do each action. An action that is not here is not
// allowed to anyone, and so is a role that is not listed for it.
var table = map[Action][]Role{
	WriteInOpenChannel: {Resident, Moderator, Administrator},
	RemoveContent:      {Moderator, Administrator},
	ChangeRoles:        {Administrator},
	SeeResidents:       {Administrator},
	ReadTheLog:         {Child, Resident, Moderator, Administrator},
}

// Can says whether a person with the role r may do the action a.
func Can(r Role, a Action) bool {
	for _, allowed := range table[a] {
		if r == allowed {
			return true
		}
	}
	return false
}

// Actions lists every action in the table.
func Actions() []Action {
	all := make([]Action, 0, len(table))
	for a := range table {
		all = append(all, a)
	}
	return all
}
