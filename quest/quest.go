// Package quest runs quests, dialogs and story triggers as data: quests
// with objectives and rewards, dialog trees whose options and availability
// depend on conditions, triggers that react to events, and named Go hooks
// the game registers for everything a script did in the original.
//
// Texts are i18n keys (the ui translates them). Items, NPCs, factions and
// areas are the game's string keys. The engine touches the game only
// through Env, and shows dialogs through a Presenter. The player's
// progress is a plain State, saved as JSON.
package quest

// Book is the game's quest content.
type Book struct {
	Quests   map[string]*Quest   `json:"quests"`
	Dialogs  map[string]*Dialog  `json:"dialogs"`
	Triggers []*Trigger          `json:"triggers,omitempty"`
	Areas    map[string]Area     `json:"areas,omitempty"`
	NPCs     map[string]*NPCTalk `json:"npcs,omitempty"`
}

// Area is a box in world blocks (Min inclusive, Max exclusive).
type Area struct {
	Min [3]float64 `json:"min"`
	Max [3]float64 `json:"max"`
}

// Contains reports whether the point is inside.
func (a Area) Contains(x, y, z float64) bool {
	return x >= a.Min[0] && x < a.Max[0] && y >= a.Min[1] && y < a.Max[1] && z >= a.Min[2] && z < a.Max[2]
}

// ObjectiveKind is what an objective asks for.
type ObjectiveKind string

const (
	Talk    ObjectiveKind = "talk"    // read dialog Target (CustomNPC dialog quest)
	Reach   ObjectiveKind = "reach"   // enter area Target (location quest)
	Kill    ObjectiveKind = "kill"    // kill Count actors of kind Target
	Collect ObjectiveKind = "collect" // hold Count of item Target (checked live)
	Deliver ObjectiveKind = "deliver" // like Collect; the items are taken on completion
	Script  ObjectiveKind = "script"  // done when the game calls Engine.Advance(quest, index)
	Counted ObjectiveKind = "event"   // Count events of kind Target (an Event's Kind:Subject)
)

// Objective is one step of a quest.
type Objective struct {
	Kind   ObjectiveKind `json:"kind"`
	Target string        `json:"target"`
	Count  int           `json:"count,omitempty"` // 0 means 1
	Text   string        `json:"text,omitempty"`  // i18n key for the quest log
}

func (o Objective) need() int {
	if o.Count <= 0 {
		return 1
	}
	return o.Count
}

// Quest is one quest.
type Quest struct {
	ID    string `json:"id"`
	Title string `json:"title"` // i18n keys
	Text  string `json:"text,omitempty"`
	Done  string `json:"done,omitempty"` // text shown on completion
	// Category groups quests in the log (a quest line).
	Category   string      `json:"category,omitempty"`
	Objectives []Objective `json:"objectives,omitempty"`
	// TurnIn names the NPC that completes the quest once every objective
	// is met (the player talks to it); "" completes it at once.
	TurnIn     string   `json:"turnIn,omitempty"`
	Rewards    []Action `json:"rewards,omitempty"`
	OnStart    []Action `json:"onStart,omitempty"`
	Next       string   `json:"next,omitempty"` // started when this one completes
	Repeatable bool     `json:"repeatable,omitempty"`
}

// Dialog is one page of a conversation.
type Dialog struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
	Text  string `json:"text"` // i18n key
	// When must hold for the dialog to be offered (CustomNPC availability).
	When    []Cond   `json:"when,omitempty"`
	Options []Option `json:"options,omitempty"`
	// Quest is started when the dialog is shown (CustomNPC DialogQuest).
	Quest string `json:"quest,omitempty"`
	// Do runs when the dialog is shown (DialogCommand, faction points).
	Do []Action `json:"do,omitempty"`
	// NoEscape: the player must pick an option.
	NoEscape bool `json:"noEscape,omitempty"`
}

// OptionKind is what picking an option does.
type OptionKind string

const (
	Close    OptionKind = "close"    // end the conversation
	Goto     OptionKind = "dialog"   // show dialog To
	Disabled OptionKind = "disabled" // shown, not pickable
	Role     OptionKind = "role"     // open the NPC's role screen (trader, bank...): Env.Role
	Command  OptionKind = "command"  // run Do, then close
)

// Option is one answer.
type Option struct {
	Text  string     `json:"text"` // i18n key
	Kind  OptionKind `json:"kind"`
	To    string     `json:"to,omitempty"`
	When  []Cond     `json:"when,omitempty"` // hidden unless all hold
	Do    []Action   `json:"do,omitempty"`   // runs when picked, before Goto/Close
	Color uint32     `json:"color,omitempty"`
}

// NPCTalk is what an NPC says when the player talks to it: the first
// available of Dialogs, else Line (a plain line, no options).
type NPCTalk struct {
	Dialogs []string `json:"dialogs,omitempty"`
	Lines   []string `json:"lines,omitempty"` // i18n keys of interact lines, picked at random
}

// Cond is one condition; every field that is set must hold (AND). Any
// holds when one of its conditions does; Not negates the whole term.
type Cond struct {
	// Quest state: "active", "done", "notStarted" (neither active nor
	// done), "notActive", "notDone", "ready" (active, objectives met).
	Quest      string `json:"quest,omitempty"`
	QuestState string `json:"questState,omitempty"`
	// Dialog read before (Read) or not (Unread).
	Dialog string `json:"dialog,omitempty"`
	Unread bool   `json:"unread,omitempty"`
	// Item: the player holds at least Count (0 means 1).
	Item  string `json:"item,omitempty"`
	Count int    `json:"count,omitempty"`
	// Faction: the player's stance toward it is Stance ("friendly",
	// "neutral", "hostile").
	Faction string `json:"faction,omitempty"`
	Stance  string `json:"stance,omitempty"`
	// Time: "day" or "night".
	Time string `json:"time,omitempty"`
	// Flag: the story flag is at least Value (0 means 1).
	Flag  string `json:"flag,omitempty"`
	Value int    `json:"value,omitempty"`
	// Script: a registered Go predicate with Args.
	Script string   `json:"script,omitempty"`
	Args   []string `json:"args,omitempty"`

	Any []Cond `json:"any,omitempty"`
	Not bool   `json:"not,omitempty"`
}

// Action is one effect. Built in: "startQuest" [id], "completeQuest" [id],
// "dropQuest" [id], "advance" [quest, objective index], "setFlag" [name,
// value], "addFlag" [name, n], "dialog" [id], "close". Anything else is a
// script hook registered with Engine.Register, called with Args (giving
// items, faction points, time of day, scenes, travel...).
type Action struct {
	Do   string   `json:"do"`
	Args []string `json:"args,omitempty"`
}

// Trigger runs Do when its event happens and When holds. On is an event
// kind ("area", "time", "pickup", "interact", "block", "kill", "dialog",
// "questStart", "questDone", or the game's own); Subject narrows it (the
// area, item, NPC, block key, dialog or quest; "" = any). An "area"
// trigger fires on entering its area; a "time" trigger fires once each
// time the clock passes Subject (a tick of the day as a number). Once
// triggers fire a single time per game.
type Trigger struct {
	ID      string   `json:"id"`
	On      string   `json:"on"`
	Subject string   `json:"subject,omitempty"`
	When    []Cond   `json:"when,omitempty"`
	Do      []Action `json:"do"`
	Once    bool     `json:"once,omitempty"`
}
