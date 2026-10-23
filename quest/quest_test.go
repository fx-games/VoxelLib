package quest

import (
	"encoding/json"
	"testing"
)

type env struct {
	items  map[string]int
	time   int
	stance map[string]string
	roles  []string
}

func (e *env) Count(item string) int { return e.items[item] }
func (e *env) Take(item string, n int) bool {
	if e.items[item] < n {
		return false
	}
	e.items[item] -= n
	return true
}
func (e *env) Time() int              { return e.time }
func (e *env) Stance(f string) string { return e.stance[f] }
func (e *env) Role(npc string)        { e.roles = append(e.roles, npc) }

type ui struct {
	views  []View
	closed int
}

func (u *ui) ShowDialog(v View) { u.views = append(u.views, v) }
func (u *ui) CloseDialog()      { u.closed++ }

// A small quest line in the shape of the converted map data.
func book() *Book {
	return &Book{
		Quests: map[string]*Quest{
			"contract": {ID: "contract", Title: "q.contract", Objectives: []Objective{
				{Kind: Talk, Target: "d.sign"}, {Kind: Deliver, Target: "item:contract", Count: 1}},
				TurnIn: "wizard", Rewards: []Action{{Do: "give", Args: []string{"item:gold", "10"}}}, Next: "trolls"},
			"trolls": {ID: "trolls", Title: "q.trolls", Objectives: []Objective{
				{Kind: Reach, Target: "camp"}, {Kind: Kill, Target: "troll", Count: 3}}},
		},
		Dialogs: map[string]*Dialog{
			"d.hello": {ID: "d.hello", Text: "t.hello", When: []Cond{{Quest: "contract", QuestState: "notStarted"}},
				Options: []Option{
					{Text: "o.adventure", Kind: Goto, To: "d.offer"},
					{Text: "o.night", Kind: Goto, To: "d.night"},
					{Text: "o.trade", Kind: Role},
					{Text: "o.bye", Kind: Close}}},
			"d.offer": {ID: "d.offer", Text: "t.offer", Quest: "contract", Options: []Option{
				{Text: "o.sign", Kind: Goto, To: "d.sign"},
				{Text: "o.skip", Kind: Command, When: []Cond{{Script: "onRoad"}}, Do: []Action{{Do: "travel"}}}}},
			"d.sign":  {ID: "d.sign", Text: "t.sign", Do: []Action{{Do: "give", Args: []string{"item:contract", "1"}}}},
			"d.night": {ID: "d.night", Text: "t.night", When: []Cond{{Time: "night"}}},
			"d.later": {ID: "d.later", Text: "t.later", When: []Cond{{Any: []Cond{
				{Quest: "contract", QuestState: "active"}, {Quest: "contract", QuestState: "done"}}}}},
		},
		NPCs:  map[string]*NPCTalk{"wizard": {Dialogs: []string{"d.hello", "d.later"}}},
		Areas: map[string]Area{"camp": {Min: [3]float64{100, 0, 100}, Max: [3]float64{120, 256, 120}}},
		Triggers: []*Trigger{
			{ID: "dawn", On: "time", Subject: "0", Do: []Action{{Do: "addFlag", Args: []string{"dawns"}}}},
			{ID: "campfire", On: "area", Subject: "camp", Once: true,
				When: []Cond{{Quest: "trolls", QuestState: "active"}}, Do: []Action{{Do: "setFlag", Args: []string{"fire", "1"}}}},
		},
	}
}

func setup() (*Engine, *env, *ui) {
	en := &env{items: map[string]int{}, stance: map[string]string{}}
	u := &ui{}
	e := New(book(), en, u)
	e.Register("give", func(e *Engine, a []string) error { en.items[a[0]] += num(a[1], 1); return nil })
	e.Register("travel", func(e *Engine, a []string) error { e.S.Flags["travelled"]++; return nil })
	e.RegisterCond("onRoad", func(e *Engine, a []string) bool { return e.S.Flags["road"] > 0 })
	e.OnError = func(err error) { panic(err) }
	return e, en, u
}

func optionTexts(v View) []string {
	var s []string
	for _, o := range v.Options {
		s = append(s, o.Text)
	}
	return s
}

func TestDialogConditions(t *testing.T) {
	e, en, u := setup()
	if !e.Talk("wizard") {
		t.Fatal("no dialog")
	}
	v := u.views[len(u.views)-1]
	// the night option leads to an unavailable dialog: hidden by day
	if got := optionTexts(v); len(got) != 3 || got[0] != "o.adventure" || got[1] != "o.trade" {
		t.Fatalf("day options %v", got)
	}
	e.Close(false)
	en.time = 15000
	e.Talk("wizard")
	if got := optionTexts(u.views[len(u.views)-1]); len(got) != 4 {
		t.Fatalf("night options %v", got)
	}
	e.Choose(2) // trade: closes and opens the role screen
	if e.Talking() || len(en.roles) != 1 {
		t.Fatal("role option")
	}
	// scripted condition on an option
	e.Talk("wizard")
	e.Choose(0)
	if v := u.views[len(u.views)-1]; v.Dialog != "d.offer" || len(v.Options) != 1 {
		t.Fatalf("offer: %+v", v)
	}
	e.S.Flags["road"] = 1
	if v := e.View(); len(v.Options) != 2 {
		t.Fatalf("onRoad option not shown: %+v", v)
	}
	e.Choose(1)
	if e.S.Flags["travelled"] != 1 || e.Talking() {
		t.Fatal("command option")
	}
	// faction condition, Not and stance
	cs := []Cond{{Faction: "elves", Stance: "friendly"}, {Item: "item:ring", Not: true}}
	if e.Eval(cs) {
		t.Fatal("not friendly yet")
	}
	en.stance["elves"] = "friendly"
	if !e.Eval(cs) {
		t.Fatal("friendly, no ring")
	}
	en.items["item:ring"] = 1
	if e.Eval(cs) {
		t.Fatal("has the ring")
	}
}

func TestQuestStateMachine(t *testing.T) {
	e, en, _ := setup()
	var log []string
	e.OnChange = func(k, q string) { log = append(log, k+":"+q) }
	e.Talk("wizard")
	e.Choose(0) // → d.offer starts "contract"
	if _, ok := e.S.Active["contract"]; !ok {
		t.Fatal("contract not active")
	}
	if !e.Available("d.later") || e.Available("d.hello") {
		t.Fatal("availability by quest state")
	}
	e.Choose(0) // → d.sign: Talk objective, gives the contract
	e.Close(true)
	l := e.Log()
	if len(l) != 1 || !l[0].Ready || l[0].Objectives[1].Have != 1 {
		t.Fatalf("log %+v", l)
	}
	e.Talk("wizard") // turn in: delivers the contract, rewards, starts trolls
	if e.S.Done["contract"] != 1 || en.items["item:contract"] != 0 || en.items["item:gold"] != 10 {
		t.Fatalf("turn-in: done %d items %v", e.S.Done["contract"], en.items)
	}
	if _, ok := e.S.Active["trolls"]; !ok {
		t.Fatal("follow-up not started")
	}
	if e.Start("contract") {
		t.Fatal("a done quest restarted")
	}
	e.Close(true)
	e.Tick(110, 64, 110) // enter the camp: objective + once trigger
	e.Tick(50, 64, 50)
	e.Tick(110, 64, 110)
	if e.S.Flags["fire"] != 1 || e.S.Active["trolls"][0] != 1 {
		t.Fatalf("area: flags %v progress %v", e.S.Flags, e.S.Active["trolls"])
	}
	e.Fire(Event{Kind: "kill", Subject: "troll", N: 2})
	e.Fire(Event{Kind: "kill", Subject: "orc"})
	if e.S.Active["trolls"][1] != 2 {
		t.Fatal("kill count")
	}
	e.Fire(Event{Kind: "kill", Subject: "troll"})
	if e.S.Done["trolls"] != 1 || len(e.Log()) != 0 {
		t.Fatal("trolls not completed")
	}
	want := []string{"start:contract", "progress:contract", "done:contract", "start:trolls"}
	for i, w := range want {
		if i >= len(log) || log[i] != w {
			t.Fatalf("changes %v", log)
		}
	}
}

func TestTimeTrigger(t *testing.T) {
	e, en, _ := setup()
	en.time = 23000
	e.Tick(0, 0, 0)
	en.time = 23900
	e.Tick(0, 0, 0)
	en.time = 24100 // past midnight-sunrise (tick 0)
	e.Tick(0, 0, 0)
	en.time = 24200
	e.Tick(0, 0, 0)
	if e.S.Flags["dawns"] != 1 {
		t.Fatalf("dawn fired %d times", e.S.Flags["dawns"])
	}
}

func TestSaveRoundTrip(t *testing.T) {
	e, _, _ := setup()
	e.Talk("wizard")
	e.Choose(0)
	e.S.Flags["x"] = 3
	data, err := json.Marshal(e.S)
	if err != nil {
		t.Fatal(err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	e2, _, _ := setup()
	e2.Load(s)
	if _, ok := e2.S.Active["contract"]; !ok || !e2.S.Read["d.offer"] || e2.S.Flags["x"] != 3 || len(e2.S.Order) != 1 {
		t.Fatalf("lost state: %+v", e2.S)
	}
	// the loaded state goes on working
	e2.Fire(Event{Kind: "dialog", Subject: "d.sign"})
	if e2.S.Active["contract"][0] != 1 {
		t.Fatal("progress after load")
	}
	// and the book itself is JSON
	if _, err := json.Marshal(book()); err != nil {
		t.Fatal(err)
	}
}

func TestHaveInactive(t *testing.T) {
	e := &Engine{Book: &Book{Quests: map[string]*Quest{"q": {Objectives: []Objective{{Kind: Kill, Target: "orc", Count: 3}}}}}, S: &State{}}
	if have, need := e.Have("q", 0); have != 0 || need != 3 {
		t.Fatalf("inactive quest: have %d need %d, want 0 3", have, need)
	}
	if have, need := e.Have("missing", 0); have != 0 || need != 0 {
		t.Fatalf("unknown quest: have %d need %d", have, need)
	}
}
