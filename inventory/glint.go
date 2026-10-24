package inventory

// GlintOf, when set, decides whether a stack glints in place of the
// default (Item.Glint or Enchanted), for items that decide themselves
// (a potion by its effects, a signed book).
var GlintOf func(s *Stack) bool

// Enchanted is ItemStack.isItemEnchanted: the tag holds an "ench" list.
func (s *Stack) Enchanted() bool {
	if s == nil {
		return false
	}
	_, ok := s.Tag["ench"].([]any)
	return ok
}

// Glints reports whether s draws with the enchantment glint
// (ItemStack.hasEffect): GlintOf, else Item.Glint or Enchanted. Every
// VoxelLib draw of a stack (slot icons, items in hands, on the ground, in
// frames, worn armour) asks this; nil does not glint.
func (s *Stack) Glints() bool {
	if s == nil || s.Item == nil {
		return false
	}
	if GlintOf != nil {
		return GlintOf(s)
	}
	return s.Item.Glint || s.Enchanted()
}
