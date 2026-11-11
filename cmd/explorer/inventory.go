package main

// The player's inventory (app.Player.Inv; the app and ui/uiapp run the
// hotbar, its keys and wheel, and the inventory screen) holds the guns
// and the ammunition: every gun and ammunition of the pack is an item. A
// gun stack carries its state as TACZ keeps it on the item's NBT
// (GunItemDataAccessor): the rounds in it, the fire mode and the
// attachments. The held stack decides the gun in hand (heldChanged);
// reloads take rounds from the ammunition stacks.

import (
	"fmt"
	"log"

	"github.com/memmaker/voxellib/importer/tacz"
	"github.com/memmaker/voxellib/inventory"
	"github.com/memmaker/voxellib/mp"
	"github.com/memmaker/voxellib/ui"
	firearm "github.com/memmaker/voxellib/weapon"
)

// The tag keys of a gun stack's state (TACZ's gun item NBT); read and
// write them only through the typed accessors below.
const (
	tagGunAmmo   = "GunCurrentAmmoCount" // int
	tagGunMode   = "GunFireMode"         // firearm.FireMode
	tagGunChoice = "Attachments"         // tacz.Choice, never changed in place
)

const (
	// creativeStock is the rounds a reload may take in the sandbox, which
	// reloads without using ammunition (TACZ in creative mode).
	creativeStock = 1 << 20
	// sandboxFirstGunSlot is where the sandbox's guns start: past the
	// hotbar, which stays empty for placing blocks.
	sandboxFirstGunSlot = inventory.HotbarSlots
)

func tagValue[T any](s *inventory.Stack, key string) (T, bool) {
	v, ok := s.Tag[key].(T)
	return v, ok
}

func setTag(s *inventory.Stack, key string, v any) {
	if s.Tag == nil {
		s.Tag = map[string]any{}
	}
	s.Tag[key] = v
}

// gunAmmo is the rounds in a gun stack (false: never set, a new gun).
func gunAmmo(s *inventory.Stack) (int, bool) { return tagValue[int](s, tagGunAmmo) }
func setGunAmmo(s *inventory.Stack, n int)   { setTag(s, tagGunAmmo, n) }
func gunMode(s *inventory.Stack) (firearm.FireMode, bool) {
	return tagValue[firearm.FireMode](s, tagGunMode)
}
func setGunMode(s *inventory.Stack, m firearm.FireMode) { setTag(s, tagGunMode, m) }

// setGunChoice installs attachments c on a gun stack (a copy: stack
// copies share the stored choice, so it is never changed in place).
func setGunChoice(s *inventory.Stack, c tacz.Choice) { setTag(s, tagGunChoice, c.Clone()) }

// stackChoice is a gun stack's attachments (the gun's default until refitted).
func (g *game) stackChoice(s *inventory.Stack) tacz.Choice {
	if c, ok := tagValue[tacz.Choice](s, tagGunChoice); ok {
		return c
	}
	return g.gunPack.DefaultChoice(s.Item.Key)
}

// stackMode is a gun stack's fire mode: the one kept on it if the gun has
// it, else the gun's first.
func (g *game) stackMode(s *inventory.Stack) firearm.FireMode {
	modes := g.gunSpec(s.Item.Key).FireModes()
	if m, ok := gunMode(s); ok {
		for _, x := range modes {
			if x == m {
				return m
			}
		}
		log.Printf("gun %s: kept fire mode %v not among %v", s.Item.Key, m, modes)
	}
	return modes[0]
}

// setupItems registers the pack's guns and ammunition as items, with their
// icons for the pickups (the item renderer) and the slots (the ui).
func (g *game) setupItems(sandbox bool) {
	g.inv().Creative = sandbox
	if g.ui != nil {
		g.ui.Inventory.Item = g.slotItem
	}
	g.itemReg = inventory.NewRegistry()
	g.icons = map[*inventory.Item]*ui.Sprite{}
	if g.gunPack == nil {
		return
	}
	for _, gun := range g.gunPack.Guns {
		g.addPackItem(&inventory.Item{Key: gun.ID, MaxStack: 1}, mp.Pickup{Kind: mp.PickupGun, ID: gun.ID}, gun.Icon)
	}
	for _, a := range g.gunPack.Ammo {
		g.addPackItem(&inventory.Item{Key: a.ID, MaxStack: a.Stack}, mp.Pickup{Kind: mp.PickupAmmo, ID: a.ID}, a.Icon)
	}
	if !sandbox {
		return
	}
	for i, gun := range g.gunPack.Guns {
		if sandboxFirstGunSlot+i >= len(g.inv().Main) {
			log.Printf("sandbox: %d of %d guns fit the inventory", i, len(g.gunPack.Guns))
			break
		}
		g.inv().Set(sandboxFirstGunSlot+i, inventory.New(g.itemReg.Get(gun.ID), 1))
	}
}

func (g *game) addPackItem(it *inventory.Item, p mp.Pickup, icon string) {
	if _, err := g.itemReg.Register(it); err != nil {
		log.Printf("item %s: %v", it.Key, err)
		return
	}
	img, err := g.gunPack.Icon(icon)
	if err != nil {
		log.Printf("item %s: %v", it.Key, err)
		return
	}
	g.addItem(p.Sprite(), img)
	if g.ui != nil {
		g.icons[it] = &ui.Sprite{Tex: g.ui.Ctx.SetTexture("voxellib:item/"+p.Sprite(), img),
			W: float32(img.Bounds().Dx()), H: float32(img.Bounds().Dy())} // the whole image, drawn 16x16
	}
}

// itemName is an item's display name.
func (g *game) itemName(it *inventory.Item) string {
	if spec := g.gunSpec(it.Key); spec != nil {
		return spec.Name
	}
	return g.ammoName(it.Key)
}

// slotItem is how a slot shows a stack (ui.PlayerGUI's Item).
func (g *game) slotItem(s *inventory.Stack) ui.ItemStack {
	st := ui.ItemOf(s, g.icons[itemOf(s)])
	if !st.Empty() {
		st.Tooltip = []string{g.itemName(s.Item)}
	}
	return st
}

func itemOf(s *inventory.Stack) *inventory.Item {
	if s == nil {
		return nil
	}
	return s.Item
}

// heldGun is the held hotbar stack when it is a gun (nil: none).
func (g *game) heldGun() *inventory.Stack {
	s := g.inv().Current()
	if s == nil || g.gunSpec(s.Item.Key) == nil {
		return nil
	}
	return s
}

// inv is the player's inventory.
func (g *game) inv() *inventory.Player { return g.Player.Inv }

// heldChanged draws the gun of the newly held stack, or puts the gun away
// for an empty or ammunition slot (Hooks.Held).
func (g *game) heldChanged(*inventory.Stack) {
	target := g.gunStack
	if g.switching {
		target = g.nextStack
	}
	if want := g.heldGun(); want != target {
		g.selectGun(want)
	}
}

// ammoCount is the rounds of ammunition id carried.
func (g *game) ammoCount(id string) int {
	n := 0
	for _, s := range g.inv().Main {
		if s != nil && s.Item.Key == id {
			n += s.Count
		}
	}
	return n
}

// ammoStock is the rounds of ammunition id a reload may take: those
// carried, any in the sandbox.
func (g *game) ammoStock(id string) int {
	if g.inv().Creative {
		return creativeStock
	}
	return g.ammoCount(id)
}

// takeAmmo removes n rounds of ammunition id, in slot order.
func (g *game) takeAmmo(id string, n int) {
	for i, s := range g.inv().Main {
		if n == 0 {
			return
		}
		if s == nil || s.Item.Key != id {
			continue
		}
		k := min(n, s.Count)
		s.Count -= k
		n -= k
		if s.Count == 0 {
			g.inv().Set(i, nil)
		}
	}
}

// giveAmmo adds n rounds of ammunition id; it reports what did not fit.
func (g *game) giveAmmo(id string, n int) (left int) {
	it := g.itemReg.Get(id)
	if it == nil {
		log.Printf("no ammunition %q in this pack", id)
		return n
	}
	s := inventory.New(it, n)
	g.inv().Add(s)
	return s.Count
}

// giveGun adds a new gun (loaded, default attachments); ok false when the
// inventory is full. A gun that lands in the hotbar while the hand holds
// no usable gun is selected.
func (g *game) giveGun(id string) bool {
	it := g.itemReg.Get(id)
	if it == nil {
		log.Printf("no gun %q in this pack", id)
		return false
	}
	slot := g.inv().FirstEmpty() // a gun stacks alone: it goes there
	if !g.inv().Add(inventory.New(it, 1)) {
		return false
	}
	if slot >= 0 && slot < inventory.HotbarSlots && (!g.gunOn() || g.gunDry()) {
		g.inv().Hold(slot)
	}
	return true
}

// gunDry reports whether the gun in hand is empty with no rounds to load:
// a picked-up gun is drawn over it, as into empty hands.
func (g *game) gunDry() bool {
	return gunEmpty(&g.gun) && g.ammoCount(g.gun.Gun.Ammo) == 0
}

// takePickup gives the player what they picked up (mp.Client.OnPickup).
func (g *game) takePickup(p mp.Pickup) {
	switch p.Kind {
	case mp.PickupGun:
		spec := g.gunSpec(p.ID)
		if spec == nil {
			log.Printf("pickup: no gun %q in this pack", p.ID)
			return
		}
		if !g.giveGun(p.ID) {
			g.note("§cInventory full: " + spec.Name + " lost")
			return
		}
		g.note("§aPicked up " + spec.Name)
	case mp.PickupAmmo:
		if left := g.giveAmmo(p.ID, p.Rounds); left > 0 {
			g.note(fmt.Sprintf("§cInventory full: %d rounds of %s lost", left, g.ammoName(p.ID)))
		}
		g.note(fmt.Sprintf("§aPicked up %d rounds of %s", p.Rounds, g.ammoName(p.ID)))
	default:
		log.Printf("pickup: unknown kind %d", p.Kind)
	}
}

// loseInventory empties the hands and the pockets (a death in zombie
// defense: back to bare hands).
func (g *game) loseInventory() {
	g.takeGun(nil)
	for i := range g.inv().Main {
		g.inv().Set(i, nil)
	}
	g.inv().Cursor = nil
}
