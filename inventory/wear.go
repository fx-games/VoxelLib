package inventory

// WearNegated, when set, is how many of n points of wear on s are absorbed
// (attemptDamageItem's per-point negation checks, such as a durability
// modifier's); AddDamage applies only the rest.
var WearNegated func(s *Stack, n int) int

// ArmorPoints, when set, is an armour stack's protection in place of its
// item's Points (an armour modifier adding to it); armour totals and
// absorption use it.
var ArmorPoints func(s *Stack) int

// ArmorValue is the stack's armour points: ArmorPoints, else the item's
// Points; 0 for a stack without armour.
func (s *Stack) ArmorValue() int {
	if s == nil || s.Item.Armor == nil {
		return 0
	}
	if ArmorPoints != nil {
		return ArmorPoints(s)
	}
	return s.Item.Armor.Points
}
