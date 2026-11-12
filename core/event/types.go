package event

import "encoding/json"

// The event types, and the data each one carries. This file is the catalog:
// a type that exists in a journal somewhere is never removed from here and
// never given another meaning. A new need gets a new type.
const (
	// CommunityCreated is entry 1 of every journal: the birth of the community.
	CommunityCreated = "community.created"

	// ChannelCreated opens a channel that every resident can read.
	ChannelCreated = "channel.created"

	// AccountCreated registers a resident. The entry number is the account.
	AccountCreated = "account.created"

	// DeviceCreated registers a browser that an account uses. The entry
	// number is the device.
	DeviceCreated = "device.created"

	// MessageCreated is a message in an open channel.
	MessageCreated = "message.created"

	// MessageDeleted takes a message out of its channel. The text leaves the
	// indexes at once; the entry that holds it is erased later, see EntryErased.
	MessageDeleted = "message.deleted"

	// EntryErased records that the contents of some entries were physically
	// replaced by the erased mark in the journal.
	EntryErased = "entry.erased"
)

type CommunityCreatedData struct {
	Name string `json:"name"`
}

type ChannelCreatedData struct {
	Name string `json:"name"`
}

type AccountCreatedData struct {
	Name string `json:"name"`

	// FirstDevice says that the browser that signed up is registered in the
	// same entry, and that the entry number is the device too. Account and
	// device are born together or not at all: with two entries, a cut
	// between them left a name taken by an account nobody could open.
	FirstDevice bool `json:"first_device,omitempty"`
}

type DeviceCreatedData struct {
	Account uint64 `json:"account"`
}

type MessageCreatedData struct {
	Channel uint64 `json:"channel"`
	Text    string `json:"text"`
}

type MessageDeletedData struct {
	Channel uint64 `json:"channel"`
	Message uint64 `json:"message"` // the entry that created the message
}

type EntryErasedData struct {
	Entries []uint64 `json:"entries"`
}

// Read decodes the data of the event into v, which is a pointer to the data
// type that matches the event type.
func (e Event) Read(v any) error {
	return json.Unmarshal(e.Data, v)
}
