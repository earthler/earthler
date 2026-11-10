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
)

type CommunityCreatedData struct {
	Name string `json:"name"`
}

type ChannelCreatedData struct {
	Name string `json:"name"`
}

type AccountCreatedData struct {
	Name string `json:"name"`
}

type DeviceCreatedData struct {
	Account uint64 `json:"account"`
}

type MessageCreatedData struct {
	Channel uint64 `json:"channel"`
	Text    string `json:"text"`
}

// Read decodes the data of the event into v, which is a pointer to the data
// type that matches the event type.
func (e Event) Read(v any) error {
	return json.Unmarshal(e.Data, v)
}
