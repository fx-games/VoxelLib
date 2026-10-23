package quest

import (
	"fmt"
	"math/rand"
	"strconv"
)

// Env is the game as the quest engine sees it.
type Env interface {
	Count(item string) int        // items of a kind the player holds
	Take(item string, n int) bool // remove from the player's inventory
	Time() int                    // tick of the day, 0..23999 (0 = sunrise)
	Stance(faction string) string // "friendly", "neutral" or "hostile" toward the player
	Role(npc string)              // open the NPC's role screen (trader, bank, ...)
}

// Presenter shows conversations (the ui's dialog screen).
type Presenter interface {
	ShowDialog(v View)
	CloseDialog()
}

// View is one dialog page as the player sees it.
type View struct {
	NPC      string // the NPC's key (the ui shows its name)
	Dialog   string // dialog ID
	Title    string // i18n keys
	Text     string
	Options  []ViewOption
	NoEscape bool
}

// ViewOption is a visible answer; pass Index to Engine.Choose.
type ViewOption struct {
	Index    int
	Text     string
	Color    uint32
	Disabled bool
}

// State is the player's story progress: the save blob part of this engine.
type State struct {
	Active map[string][]int `json:"active"` // quest → progress per objective
	Done   map[string]int   `json:"done"`   // quest → times completed
	Read   map[string]bool  `json:"read"`   // dialogs seen
	Flags  map[string]int   `json:"flags"`
	Fired  map[string]bool  `json:"fired"`  // Once triggers that fired
	Inside map[string]bool  `json:"inside"` // areas the player is in
	Clock  int              `json:"clock"`  // last tick of day seen, for time triggers
	Order  []string         `json:"order"`  // active quests in start order (quest log)
}

func newState() *State {
	s := &State{}
	s.init()
	return s
}

func (s *State) init() {
	if s.Active == nil {
		s.Active = map[string][]int{}
	}
	if s.Done == nil {
		s.Done = map[string]int{}
	}
	if s.Read == nil {
		s.Read = map[string]bool{}
	}
	if s.Flags == nil {
		s.Flags = map[string]int{}
	}
	if s.Fired == nil {
		s.Fired = map[string]bool{}
	}
	if s.Inside == nil {
		s.Inside = map[string]bool{}
	}
}

// Event is something that happened in the game: Kind is "kill" (Subject:
// actor kind), "pickup" (item), "interact" (NPC), "block" (block key),
// "dialog" (dialog ID, fired by the engine), "area" (area, fired by Tick),
// "questStart"/"questDone" (fired by the engine), or the game's own.
type Event struct {
	Kind, Subject string
	N             int // how many (kills, items); 0 means 1
}

// Hook is a registered Go script: what a map script or command did.
type Hook func(e *Engine, args []string) error

// Engine runs the book for one player.
type Engine struct {
	Book *Book
	S    *State
	Env  Env
	UI   Presenter
	Rand *rand.Rand
	// OnChange is called when a quest starts, advances or completes
	// (refresh the quest log, show a message): kind is "start",
	// "progress" or "done".
	OnChange func(kind, quest string)
	// OnError reports actions that failed or have no hook.
	OnError func(error)

	hooks map[string]Hook
	conds map[string]func(e *Engine, args []string) bool
	npc   string
	cur   *Dialog
	depth int
}

// New returns an engine with a fresh State.
func New(b *Book, env Env, ui Presenter) *Engine {
	return &Engine{Book: b, S: newState(), Env: env, UI: ui, Rand: rand.New(rand.NewSource(1)),
		hooks: map[string]Hook{}, conds: map[string]func(*Engine, []string) bool{}}
}

// Register adds a script hook, run by an Action with Do == name.
func (e *Engine) Register(name string, h Hook) { e.hooks[name] = h }

// RegisterCond adds a script predicate, used by a Cond with Script == name.
func (e *Engine) RegisterCond(name string, f func(e *Engine, args []string) bool) { e.conds[name] = f }

// Load replaces the state (from a save).
func (e *Engine) Load(s State) {
	s.init()
	e.S = &s
}

func (e *Engine) fail(err error) {
	if e.OnError != nil {
		e.OnError(err)
	}
}

func (e *Engine) changed(kind, q string) {
	if e.OnChange != nil {
		e.OnChange(kind, q)
	}
}

// ---- conditions ----

// Eval reports whether every condition holds.
func (e *Engine) Eval(cs []Cond) bool {
	for _, c := range cs {
		if !e.eval(c) {
			return false
		}
	}
	return true
}

func (e *Engine) eval(c Cond) bool {
	ok := e.term(c)
	if c.Not {
		return !ok
	}
	return ok
}

func (e *Engine) term(c Cond) bool {
	if c.Quest != "" {
		_, active := e.S.Active[c.Quest]
		done := e.S.Done[c.Quest] > 0
		var ok bool
		switch c.QuestState {
		case "active":
			ok = active
		case "done", "":
			ok = done
		case "notStarted":
			ok = !active && !done
		case "notActive":
			ok = !active
		case "notDone":
			ok = !done
		case "ready":
			ok = active && e.met(c.Quest)
		}
		if !ok {
			return false
		}
	}
	if c.Dialog != "" && e.S.Read[c.Dialog] == c.Unread {
		return false
	}
	if c.Item != "" && e.Env.Count(c.Item) < max(1, c.Count) {
		return false
	}
	if c.Faction != "" && e.Env.Stance(c.Faction) != c.Stance {
		return false
	}
	if c.Time != "" && (c.Time == "night") != IsNight(e.Env.Time()) {
		return false
	}
	if c.Flag != "" && e.S.Flags[c.Flag] < max(1, c.Value) {
		return false
	}
	if c.Script != "" {
		f := e.conds[c.Script]
		if f == nil {
			e.fail(fmt.Errorf("quest: no condition %q", c.Script))
			return false
		}
		if !f(e, c.Args) {
			return false
		}
	}
	if len(c.Any) > 0 {
		for _, a := range c.Any {
			if e.eval(a) {
				return true
			}
		}
		return false
	}
	return true
}

// IsNight is Minecraft's night for a tick of the day (CustomNPC: 13000..23000).
func IsNight(t int) bool { t %= 24000; return t >= 13000 && t < 23000 }

// ---- actions ----

// Run executes actions in order.
func (e *Engine) Run(as []Action) {
	e.depth++
	defer func() { e.depth-- }()
	if e.depth > 32 {
		e.fail(fmt.Errorf("quest: actions nest too deep"))
		return
	}
	for _, a := range as {
		e.run(a)
	}
}

func arg(a []string, i int) string {
	if i < len(a) {
		return a[i]
	}
	return ""
}

func num(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func (e *Engine) run(a Action) {
	switch a.Do {
	case "startQuest":
		e.Start(arg(a.Args, 0))
	case "completeQuest":
		e.Complete(arg(a.Args, 0))
	case "dropQuest":
		e.Drop(arg(a.Args, 0))
	case "advance":
		e.Advance(arg(a.Args, 0), num(arg(a.Args, 1), 0))
	case "setFlag":
		e.S.Flags[arg(a.Args, 0)] = num(arg(a.Args, 1), 1)
	case "addFlag":
		e.S.Flags[arg(a.Args, 0)] += num(arg(a.Args, 1), 1)
	case "dialog":
		e.Open(e.npc, arg(a.Args, 0))
	case "close":
		e.Close(true)
	default:
		h := e.hooks[a.Do]
		if h == nil {
			e.fail(fmt.Errorf("quest: no hook %q", a.Do))
			return
		}
		if err := h(e, a.Args); err != nil {
			e.fail(fmt.Errorf("quest: %s %v: %w", a.Do, a.Args, err))
		}
	}
}

// ---- quests ----

// Start begins a quest. It fails if the quest is active, or done and not
// repeatable.
func (e *Engine) Start(id string) bool {
	q := e.Book.Quests[id]
	if q == nil {
		e.fail(fmt.Errorf("quest: no quest %q", id))
		return false
	}
	if _, ok := e.S.Active[id]; ok || (e.S.Done[id] > 0 && !q.Repeatable) {
		return false
	}
	prog := make([]int, len(q.Objectives))
	for i, o := range q.Objectives { // a dialog read before counts
		if o.Kind == Talk && e.S.Read[o.Target] {
			prog[i] = 1
		}
	}
	e.S.Active[id] = prog
	e.S.Order = append(e.S.Order, id)
	e.Run(q.OnStart)
	e.changed("start", id)
	e.Fire(Event{Kind: "questStart", Subject: id})
	e.check(id)
	return true
}

// Drop abandons an active quest.
func (e *Engine) Drop(id string) {
	if _, ok := e.S.Active[id]; ok {
		delete(e.S.Active, id)
		e.unorder(id)
		e.changed("drop", id)
	}
}

func (e *Engine) unorder(id string) {
	for i, o := range e.S.Order {
		if o == id {
			e.S.Order = append(e.S.Order[:i], e.S.Order[i+1:]...)
			return
		}
	}
}

// Have returns an objective's progress and need (Collect and Deliver count
// what the player holds now).
func (e *Engine) Have(id string, i int) (have, need int) {
	q := e.Book.Quests[id]
	if q == nil || i < 0 || i >= len(q.Objectives) {
		return 0, 0
	}
	o := q.Objectives[i]
	need = o.need()
	if o.Kind == Collect || o.Kind == Deliver {
		return min(need, e.Env.Count(o.Target)), need
	}
	if p := e.S.Active[id]; i < len(p) {
		return min(need, p[i]), need
	}
	return 0, need
}

// met reports whether every objective of an active quest is met.
func (e *Engine) met(id string) bool {
	q := e.Book.Quests[id]
	if q == nil {
		return false
	}
	if _, ok := e.S.Active[id]; !ok {
		return false
	}
	for i := range q.Objectives {
		if h, n := e.Have(id, i); h < n {
			return false
		}
	}
	return true
}

// check completes a quest without a TurnIn NPC once it is met.
func (e *Engine) check(id string) {
	if q := e.Book.Quests[id]; q != nil && q.TurnIn == "" && e.met(id) {
		e.Complete(id)
	}
}

// Advance adds one to a Script (or any counted) objective.
func (e *Engine) Advance(id string, i int) {
	p, ok := e.S.Active[id]
	if !ok || i < 0 || i >= len(p) {
		return
	}
	p[i]++
	e.changed("progress", id)
	e.check(id)
}

// Complete finishes an active quest: delivered items are taken, rewards
// run, the follow-up starts.
func (e *Engine) Complete(id string) {
	q := e.Book.Quests[id]
	if _, ok := e.S.Active[id]; !ok || q == nil {
		return
	}
	for _, o := range q.Objectives {
		if o.Kind == Deliver {
			e.Env.Take(o.Target, o.need())
		}
	}
	delete(e.S.Active, id)
	e.unorder(id)
	e.S.Done[id]++
	e.Run(q.Rewards)
	e.changed("done", id)
	e.Fire(Event{Kind: "questDone", Subject: id})
	if q.Next != "" {
		e.Start(q.Next)
	}
}

// ---- events and triggers ----

// Fire feeds an event to the active quests' objectives and the triggers.
func (e *Engine) Fire(ev Event) {
	n := max(1, ev.N)
	var touched []string
	for id, prog := range e.S.Active {
		q := e.Book.Quests[id]
		if q == nil {
			continue
		}
		hit := false
		for i, o := range q.Objectives {
			match := false
			switch o.Kind {
			case Talk:
				match = ev.Kind == "dialog" && ev.Subject == o.Target
			case Reach:
				match = ev.Kind == "area" && ev.Subject == o.Target
			case Kill:
				match = ev.Kind == "kill" && ev.Subject == o.Target
			case Counted:
				match = o.Target == ev.Kind+":"+ev.Subject || o.Target == ev.Kind
			}
			if match && prog[i] < o.need() {
				prog[i] = min(o.need(), prog[i]+n)
				hit = true
			}
		}
		if hit || ev.Kind == "pickup" {
			touched = append(touched, id)
		}
	}
	for _, id := range touched {
		e.changed("progress", id)
		e.check(id)
	}
	for _, t := range e.Book.Triggers {
		if t.On == ev.Kind && (t.Subject == "" || t.Subject == ev.Subject) {
			e.trigger(t)
		}
	}
}

func (e *Engine) trigger(t *Trigger) {
	if (t.Once && e.S.Fired[t.ID]) || !e.Eval(t.When) {
		return
	}
	if t.Once {
		e.S.Fired[t.ID] = true
	}
	e.Run(t.Do)
}

// Tick checks the player's position against the areas (entering one fires
// an "area" event) and the clock against "time" triggers. Call it every
// tick or every few.
func (e *Engine) Tick(x, y, z float64) {
	for id, a := range e.Book.Areas {
		in := a.Contains(x, y, z)
		if in && !e.S.Inside[id] {
			e.S.Inside[id] = true
			e.Fire(Event{Kind: "area", Subject: id})
		} else if !in && e.S.Inside[id] {
			delete(e.S.Inside, id)
		}
	}
	now := e.Env.Time() % 24000
	prev := e.S.Clock
	e.S.Clock = now
	if now == prev {
		return
	}
	for _, t := range e.Book.Triggers {
		if t.On != "time" {
			continue
		}
		at := num(t.Subject, -1)
		if at < 0 {
			continue
		}
		// passed when at is in (prev, now], across midnight too
		if (prev < now && at > prev && at <= now) || (prev > now && (at > prev || at <= now)) {
			e.trigger(t)
		}
	}
}

// ---- dialogs ----

// Available reports whether a dialog may be offered now.
func (e *Engine) Available(id string) bool {
	d := e.Book.Dialogs[id]
	return d != nil && e.Eval(d.When)
}

// Talk is the player speaking to an NPC: quests waiting for this NPC
// complete, then the first available of its dialogs opens. It returns
// false when the NPC has no dialog to offer (the game may show Line).
func (e *Engine) Talk(npc string) bool {
	for _, id := range append([]string(nil), e.S.Order...) {
		if q := e.Book.Quests[id]; q != nil && q.TurnIn == npc && e.met(id) {
			e.Complete(id)
		}
	}
	e.Fire(Event{Kind: "interact", Subject: npc})
	t := e.Book.NPCs[npc]
	if t == nil {
		return false
	}
	for _, id := range t.Dialogs {
		if e.Available(id) {
			e.Open(npc, id)
			return true
		}
	}
	return false
}

// Line picks one of the NPC's interact lines (i18n key), or "".
func (e *Engine) Line(npc string) string {
	if t := e.Book.NPCs[npc]; t != nil && len(t.Lines) > 0 {
		return t.Lines[e.Rand.Intn(len(t.Lines))]
	}
	return ""
}

// Open shows a dialog: it is marked read, its quest starts, its actions
// run, and the presenter gets the page.
func (e *Engine) Open(npc, id string) {
	d := e.Book.Dialogs[id]
	if d == nil {
		e.fail(fmt.Errorf("quest: no dialog %q", id))
		e.Close(true)
		return
	}
	e.npc, e.cur = npc, d
	e.S.Read[id] = true
	if d.Quest != "" {
		e.Start(d.Quest)
	}
	e.Run(d.Do)
	e.Fire(Event{Kind: "dialog", Subject: id})
	if e.cur == d && e.UI != nil {
		e.UI.ShowDialog(e.View())
	}
}

// View is the open page, or a zero View when no dialog is open.
func (e *Engine) View() View {
	d := e.cur
	if d == nil {
		return View{}
	}
	v := View{NPC: e.npc, Dialog: d.ID, Title: d.Title, Text: d.Text, NoEscape: d.NoEscape}
	for i, o := range d.Options {
		if !e.Eval(o.When) {
			continue
		}
		if o.Kind == Goto && !e.Available(o.To) {
			continue // CustomNPC hides options to dialogs that aren't available
		}
		v.Options = append(v.Options, ViewOption{Index: i, Text: o.Text, Color: o.Color, Disabled: o.Kind == Disabled})
	}
	return v
}

// Choose picks option i (ViewOption.Index) of the open dialog.
func (e *Engine) Choose(i int) {
	d := e.cur
	if d == nil || i < 0 || i >= len(d.Options) {
		return
	}
	o := d.Options[i]
	if o.Kind == Disabled || !e.Eval(o.When) {
		return
	}
	e.Run(o.Do)
	if e.cur != d { // an action opened another dialog or closed this one
		return
	}
	switch o.Kind {
	case Goto:
		e.Open(e.npc, o.To)
	case Role:
		npc := e.npc
		e.Close(true)
		e.Env.Role(npc)
	default:
		e.Close(true)
	}
}

// Close ends the conversation; without force a NoEscape dialog stays.
func (e *Engine) Close(force bool) {
	if e.cur == nil || (!force && e.cur.NoEscape) {
		return
	}
	e.cur, e.npc = nil, ""
	if e.UI != nil {
		e.UI.CloseDialog()
	}
}

// Talking reports whether a conversation is open.
func (e *Engine) Talking() bool { return e.cur != nil }

// ---- quest log ----

// LogEntry is one active quest for the quest log screen.
type LogEntry struct {
	ID, Title, Text, Category string
	Objectives                []LogObjective
	Ready                     bool // met, waiting for the TurnIn NPC
	TurnIn                    string
}

// LogObjective is one objective's line.
type LogObjective struct {
	Text       string
	Have, Need int
}

// Log lists the active quests in the order they started.
func (e *Engine) Log() []LogEntry {
	var out []LogEntry
	for _, id := range e.S.Order {
		q := e.Book.Quests[id]
		if q == nil {
			continue
		}
		le := LogEntry{ID: id, Title: q.Title, Text: q.Text, Category: q.Category, TurnIn: q.TurnIn, Ready: e.met(id)}
		for i, o := range q.Objectives {
			h, n := e.Have(id, i)
			le.Objectives = append(le.Objectives, LogObjective{Text: o.Text, Have: h, Need: n})
		}
		out = append(out, le)
	}
	return out
}
