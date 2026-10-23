package png_chunks

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A text chunk long enough to be worth reading back: a whole ZDoom weapon
// script, which is the thing this actually carries - see chunk_text.go.
//
// The PNG it is injected into is made here rather than read from assets/. It
// used to be a checked-in file, written back out beside itself as
// assets/test_text.png - a test that leaves scratch files in the game's asset
// folder, and one that stops compiling the day that folder is tidied up. Which
// is what happened.
func TestReadTextChunkFromPNG(t *testing.T) {
	var source bytes.Buffer
	if err := png.Encode(&source, testImage()); err != nil {
		t.Fatalf("encoding the source png: %s", err)
	}

	var withText bytes.Buffer
	if err := InjectTextChunkIntoPNG(bytes.NewReader(source.Bytes()), &withText, "AtlasInfo", longText); err != nil {
		t.Fatalf("injecting the chunk: %s", err)
	}

	assert.Equal(t, longText, ReadTextChunkFromPNG(bytes.NewReader(withText.Bytes()), "AtlasInfo"))

	// The picture has to survive the trip: a chunk written into the wrong place
	// reads back fine and leaves an image no decoder will open.
	decoded, err := png.Decode(bytes.NewReader(withText.Bytes()))
	if err != nil {
		t.Fatalf("the png with a chunk in it no longer decodes: %s", err)
	}
	if got := decoded.Bounds(); got != testImage().Bounds() {
		t.Errorf("decoded bounds %v, want %v", got, testImage().Bounds())
	}
}

// testImage is a few pixels of something, so the injected chunk has real image
// data on both sides of it rather than an empty file.
func testImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 32), G: uint8(y * 32), B: 128, A: 255})
		}
	}
	return img
}

const longText = `
SelectAnimation:
	TNT1 AA 1 A_JumpIfInventory("GoFatality", 1, "Steady")
	A12G A 0 A_GunFlash
	A12G A 0 A_JumpIfInventory("AutoShotUnloaded", 1, "ReadyNoFuckingAmmo")
	A12G A 0 A_PlaySound("CLIPIN")
	A12S ABCD 1 
Ready:
Ready3:
	A12G A 0 
	A12G A 1 BRIGHT A_WeaponReady(WRF_ALLOWRELOAD)
	A12G A 0 A_JumpIfInventory("Kicking",1,"DoKick")
	A12G A 0 A_JumpIfInventory("Taunting",1,"Taunt")
	A12G A 0 A_JumpIfInventory("Salute1", 1, "Salute")
	A12G A 0 A_JumpIfInventory("Salute2", 1, "Salute")
	A12G A 0 A_JumpIfInventory("Reloading",1,"Reload")
	A12G A 0 A_JumpIfInventory("Unloading",1,"Unload")
	TNT1 A 0 A_JumpIfInventory("weargasmask", 1, "wearanimation")
	TNT1 A 0 A_JumpIfInventory("removegasmask", 1, "removeanimation")
	TNT1 A 0 A_JumpIfInventory("battledrugtoken",1,"drugshot")
	TNT1 A 0 A_JumpIfInventory("BANDAGETOKEN",1,"BANDAGE")
	TNT1 A 0 A_Jumpifinventory("throwtoken",1, "Quickthrow")
	TNT1 A 0 A_Jumpifinventory("slashtoken",1, "Quickslash")
	TNT1 A 0 A_JumpIfInventory("flaretoken",1,"TossFlare")
	A12G A 0 A_JumpIfInventory("TossGrenade",1,"TossGrenade")
	A12G A 0 A_JumpIfInventory("IsRunning",1,"CheckSprint")
	A12G A 0 A_JumpIfInventory("Zoomed",1,"Ready2")
	A12G A 0 A_JumpIfInventory("FiredPrimary",1,"Fire")
	A12G A 0 A_JumpIfInventory("AutoShotUnloaded", 1, "ReadyNoFuckingAmmo")
	Loop
Ready2:
	A12A D 1 A_WeaponReady(WRF_ALLOWRELOAD)
	A12G A 0 A_JumpIfInventory("Kicking",1,"DoKick")
	A12G A 0 A_JumpIfInventory("Taunting",1,"Taunt")
	A12G A 0 A_JumpIfInventory("Salute1", 1, "Salute")
	A12G A 0 A_JumpIfInventory("Salute2", 1, "Salute")
	A12G A 0 A_JumpIfInventory("Reloading",1,"Reload")
	A12G A 0 A_JumpIfInventory("Unloading",1,"Unload")
	TNT1 A 0 A_JumpIfInventory("weargasmask", 1, "wearanimation")
	TNT1 A 0 A_JumpIfInventory("removegasmask", 1, "removeanimation")
	TNT1 A 0 A_JumpIfInventory("battledrugtoken",1,"drugshot")
	TNT1 A 0 A_JumpIfInventory("BANDAGETOKEN",1,"BANDAGE")
	TNT1 A 0 A_Jumpifinventory("throwtoken",1, "Quickthrow")
	TNT1 A 0 A_Jumpifinventory("slashtoken",1, "Quickslash")
	TNT1 A 0 A_JumpIfInventory("flaretoken",1,"TossFlare")
	A12G A 0 A_JumpIfInventory("TossGrenade",1,"TossGrenade")
	A12G A 0 A_JumpIfInventory("IsRunning",1,"CheckSprint")
	A12G A 0 A_JumpIfInventory("FiredPrimary",1,"Fire")
	Loop
ReadyNoFuckingAmmo:
	A12R Q 1 BRIGHT A_WeaponReady(WRF_ALLOWRELOAD)
	A12G A 0 A_JumpIfInventory("Kicking",1,"DoKick")
	A12G A 0 A_JumpIfInventory("Taunting",1,"Taunt")
	A12G A 0 A_JumpIfInventory("Salute1", 1, "Salute")
	A12G A 0 A_JumpIfInventory("Salute2", 1, "Salute")
	A12G A 0 A_JumpIfInventory("Reloading",1,"Reload")
	A12G A 0 A_JumpIfInventory("Unloading",1,"Unload")
	TNT1 A 0 A_JumpIfInventory("weargasmask", 1, "wearanimation")
	TNT1 A 0 A_JumpIfInventory("removegasmask", 1, "removeanimation")
	TNT1 A 0 A_JumpIfInventory("battledrugtoken",1,"drugshot")
	TNT1 A 0 A_JumpIfInventory("BANDAGETOKEN",1,"BANDAGE")
	TNT1 A 0 A_Jumpifinventory("throwtoken",1, "Quickthrow")
	TNT1 A 0 A_Jumpifinventory("slashtoken",1, "Quickslash")
	TNT1 A 0 A_JumpIfInventory("flaretoken",1,"TossFlare")
	A12G A 0 A_JumpIfInventory("TossGrenade",1,"TossGrenade")
	A12G A 0 A_JumpIfInventory("IsRunning",1,"CheckSprint")
	Loop
Deselect:
	A12G A 0 
	A12G A 0 A_Takeinventory("Zoomed",1)
	A12G A 0 A_Takeinventory("ADSmode",1)
	TNT1 A 0 A_TakeInventory("throwtoken", 1)
	TNT1 A 0 A_TakeInventory("slashtoken", 1)
	DURI A 0 A_TakeInventory("Flaretoken", 1)
	A12G A 0 A_TakeInventory("TossGrenade", 1)
	A12G A 0 A_ZoomFactor(1.0)
	A12G A 0 A_JumpIfInventory("AutoShotUnloaded", 1, "DeSelectNoAmmo")
	A12S DCBA 1 
	TNT1 AAAAAAAAAAAAAAAAAA 0 A_Lower
	TNT1 A 1 A_Lower
	Wait
DeSelectNoAmmo:
	A12R Q 1 A_Lower
	Wait
Select:
	TNT1 A 0 SetPlayerProperty(0,0,2)
	TNT1 A 0 A_TakeInventory("raredropselected",1)
	A12G A 0 A_Giveinventory("GoSpecial",1)
	A12G A 0 A_Takeinventory("FistsSelected",1)
	A12G A 0 A_Takeinventory("SawSelected",1)
	A12G A 0 A_Takeinventory("ShotgunSelected",1)
	A12G A 0 A_Giveinventory("SSGSelected",1)
	A12G A 0 A_Takeinventory("MinigunSelected",1)
	A12G A 0 A_Takeinventory("PlasmaGunSelected",1)
	A12G A 0 A_Takeinventory("RocketLauncherSelected",1)
	A12G A 0 A_Takeinventory("GrenadeLauncherSelected",1)
	A12G A 0 A_Takeinventory("BFGSelected",1)
	A12G A 0 A_Takeinventory("BFG10kSelected",1)
	A12G A 0 A_Takeinventory("RailGunSelected",1)
	A12G A 0 A_Takeinventory("SubMachineGunSelected",1)
	A12G A 0 A_Takeinventory("RevenantLauncherSelected",1)
	A12G A 0 A_Takeinventory("LostSoulSelected",1)
	A12G A 0 A_Takeinventory("FlameCannonSelected",1)
	A12G A 0 A_Takeinventory("HasBarrel",1)
	DURI A 0 A_TakeInventory("Flaretoken", 1)
	A12G A 0 A_TakeInventory("TossGrenade", 1)
	A12G A 0 A_JumpIfInventory("AutoShotUnloaded", 1, "SelectsNoAmmo")
	TNT1 A 1 A_Raise
	TNT1 AAAAAAAAA 0 A_Raise
	Goto SelectAnimation
SelectsNoAmmo:
	A12R QQQQQQ 1 BRIGHT A_Raise
	Goto SelectAnimation
Fire:
	A12G A 0 A_JumpIfInventory("AssaultShotgunAmmo",1,2)
	Goto NoAmmo
	TNT1 AAAA 0 
	A12G A 0 A_PlaySound("A12FIR", 1)
	RIFF A 0 A_FireCustomMissile("DistantFireSoundShotgun", random(-1,1), 0, 0, -12, 0, random(-1,1))
	TNT1 AAAAAAAAAAAA 0 BRIGHT A_FireCustomMissile("ShotgunParticles", random(-12,12), 0, -1, 0, 0, random(-9,9))
	TNT1 AAA 0 BRIGHT A_FireCustomMissile("ShotgunParticles2", random(-12,12), 0, -1, 0, 0, random(-9,9))
	A12G A 0 A_JumpIfInventory("Zoomed",1,"Fire2")
	A12F A 1 BRIGHT A_AlertMonsters
	A12G A 0 A_SpawnItemEx("PlayerMuzzle1",32,0,28)
	A12F A 0 BRIGHT A_FireBullets (5, 5, 10, 14, "Shotgunpuff", FBF_NORANDOM)
	RIFF AAAAAAAA 0 A_FireCustomMissile("DecorativeTracer", random(-4,4), 0, -1, -12, 0, random(-4,4))
	A12G A 0 
	A12G A 0 A_JumpIfInventory("IsNOTTacticalClass", 1, 3)
	SHTN A 0 A_FireBullets (4, 4, 6, 10, "ShotgunPuff2", FBF_NORANDOM)
	A12G A 0 A_JumpIfInventory("IsNOTTacticalClass", 1, 6)
	RIFG A 0 A_JumpIfInventory("norecoil",1,5)
	A12G A 0 A_SetPitch(pitch-6)
	A12G A 0 
	TNT1 AA 0 A_SetANgle(angle+random(-1, 1))
	A12G A 0 A_ZoomFactor(0.9)
	A12G A 0 
	A12G A 0 
	A12G A 0 A_JumpIfInventory("IsNOTTacticalClass", 1, 4)
	RIFG A 0 A_JumpIfInventory("norecoil",1,3)
	A12G A 0 A_SetPitch(-1.2 + pitch)
	A12G A 0 
	A12G A 0 A_Takeinventory("AssaultShotgunAmmo",1)
	A12G A 0 A_JumpIfInventory("IsNOTTacticalClass", 1, 4)
	RIFG A 0 A_JumpIfInventory("norecoil",1,3)
	A12G A 0 A_SetPitch(+0.6 + pitch)
	A12G A 0 
	A12F B 1 A_FireCustomMissile("ShotCaseSpawn",5,0,6,-14)
	A12G A 0 A_ZoomFactor(1.0)
	A12G A 0 A_JumpIfInventory("IsNOTTacticalClass", 1, 4)
	RIFG A 0 A_JumpIfInventory("norecoil",1,3)
	A12F C 1 A_SetPitch(+0.6 + pitch)
	A12G A 0 
	A12F DEF 1 
	A12G A 0 A_JumpIfInventory("FiredPrimary",1,"Fire")
	SHTZ A 0 A_Refire
	A12G A 5 A_WeaponReady(1)
	Goto Ready
Fire2:
	A12A E 1 BRIGHT A_AlertMonsters
	A12G A 0 A_SpawnItemEx("PlayerMuzzle1",32,0,28)
	A12F A 0 BRIGHT A_FireBullets (5, 3, 10, 13, "ShotgunPuff", FBF_NORANDOM)
	RIFF AAAAAAAA 0 A_FireCustomMissile("DecorativeTracer", random(-4,4), 0, -1, -12, 0, random(-4,4))
	A12G A 0 
	A12G A 0 A_JumpIfInventory("IsNOTTacticalClass", 1, 3)
	SHTN A 0 A_FireBullets (3, 4, 5, 13, "ShotgunPuff2", FBF_NORANDOM)
	A12G A 0 A_JumpIfInventory("IsNOTTacticalClass", 1, 4)
	RIFG A 0 A_JumpIfInventory("norecoil",1,3)
	A12G A 0 A_SetPitch(pitch-6)
	A12G A 0 
	A12G A 0 
	TNT1 AA 0 A_SetANgle(angle+random(-1, 1))
	A12G A 0 A_ZoomFactor(1.2)
	A12G A 0 A_JumpIfInventory("IsNOTTacticalClass", 1, 4)
	RIFG A 0 A_JumpIfInventory("norecoil",1,3)
	A12G A 0 A_SetPitch(-0.6 + pitch)
	A12G A 0 
	A12G A 0 A_Takeinventory("AssaultShotgunAmmo",1)
	A12G A 0 A_JumpIfInventory("IsNOTTacticalClass", 1, 4)
	RIFG A 0 A_JumpIfInventory("norecoil",1,3)
	A12G A 0 A_SetPitch(+0.3 + pitch)
	A12G A 0 
	A12A F 1 A_FireCustomMissile("ShotCaseSpawn",5,0,6,0)
	A12G A 0 A_ZoomFactor(1.3)
	A12G A 0 A_JumpIfInventory("IsNOTTacticalClass", 1, 4)
	RIFG A 0 A_JumpIfInventory("norecoil",1,3)
	A12A G 1 A_SetPitch(+0.3 + pitch)
	A12G A 0 
	A12A DDD 1 
	A12G A 0 A_JumpIfInventory("FiredPrimary",1,"Fire")
	A12G A 0 A_JumpIfInventory("FiredSecondary",1,"HoldAim")
	SHTZ A 0 A_Refire
	A12A D 5 A_WeaponReady(1)
	Goto Ready2
AltFire:
	A12G A 0 
	A12G A 0 A_Giveinventory("GoSpecial",1)
	A12G A 0 A_JumpIfInventory("Zoomed",1,"NoAim")
	A12G A 0 A_Giveinventory("Zoomed",1)
	A12G A 0 A_ZoomFactor(1.3)
	A12G A 0 A_Giveinventory("ADSmode",1)
	A12A ABC 1 
	A12G A 0 A_JumpIfInventory("FiredPrimary",1,"Fire")
	A12A D 1 A_WeaponReady(WRF_ALLOWRELOAD)
	A12G A 0 A_JumpIfInventory("FiredPrimary",1,"Fire")
	A12A D 1 A_WeaponReady(WRF_ALLOWRELOAD)
	A12G A 0 A_JumpIfInventory("FiredPrimary",1,"Fire")
	A12A D 1 A_WeaponReady(WRF_ALLOWRELOAD)
	A12G A 0 A_JumpIfInventory("FiredPrimary",1,"Fire")
	A12G A 0 A_JumpIfInventory("FiredSecondary",1,"HoldAim")
	Goto Ready2
HoldAim:
	A12G A 0 A_JumpIfInventory("FiredPrimary",1,"Fire")
	A12A D 1 A_WeaponReady(WRF_ALLOWRELOAD)
	A12G A 0 A_JumpIfInventory("FiredPrimary",1,"Fire")
	A12A D 1 A_WeaponReady(WRF_ALLOWRELOAD)
	A12G A 0 A_JumpIfInventory("FiredPrimary",1,"Fire")
	A12A D 1 A_WeaponReady(WRF_ALLOWRELOAD)
	A12G A 0 A_JumpIfInventory("FiredSecondary",1,"HoldAim")
NoAim:
	A12G A 0 A_Takeinventory("Zoomed",1)
	A12G A 0 A_ZoomFactor(1.0)
	A12G A 0 A_Takeinventory("ADSmode",1)
	A12A CBA 1 
	Goto Ready3
NoAmmo:
	RIFG A 0 
	RIFG A 0 A_ZoomFactor(1.0)
	RIFG A 0 A_Takeinventory("Zoomed",1)
	RIFG A 0 A_Takeinventory("ADSmode",1)
	RIFG A 0 A_JumpIfInventory("AutoShotUnloaded",1,"ReadyNoFuckingAmmo")
	TNT1 A 0 A_PlaySound("weapons/empty", 4)
NoAmmo2:
	TNT1 A 0 A_JumpIfInventory("weargasmask", 1, "wearanimation")
	TNT1 A 0 A_JumpIfInventory("removegasmask", 1, "removeanimation")
	TNT1 A 0 A_JumpIfInventory("battledrugtoken",1,"drugshot")
	TNT1 A 0 A_JumpIfInventory("BANDAGETOKEN",1,"BANDAGE")
	TNT1 A 0 A_Jumpifinventory("throwtoken",1, "Quickthrow")
	TNT1 A 0 A_Jumpifinventory("slashtoken",1, "Quickslash")
	TNT1 A 0 A_JumpIfInventory("flaretoken",1,"TossFlare")
	RIFG A 0 A_JumpIfInventory("TossGrenade",1,"TossGrenade")
	RIFG A 0 A_JumpIfInventory("Taunting",1,"Taunt")
	RIFG A 0 A_JumpIfInventory("Salute1", 1, "Salute")
	RIFG A 0 A_JumpIfInventory("Salute2", 1, "Salute")
	RIFG A 0 A_JumpIfInventory("IsRunning",1,"CheckSprint")
	RIFG A 0 A_JumpIfInventory("ExecuteDownedEnemy", 1, "DoExecution")
	RIFG A 0 A_JumpIfInventory("Kicking",1,"DoKick")
	A12G A 5 A_WeaponReady(WRF_ALLOWRELOAD | WRF_NOPRIMARY)
	RIFG A 0 A_JumpIfInventory("Reloading",1,"Reload")
	RIFG A 0 A_JumpIfInventory("TurboReload",1,"InsertBullets")
	TNT1 A 0 A_JumpIfInventory("NoAutoReload", 1, "NoAmmo2")
	RIFG A 0 A_JumpIfInventory("AmmoShell",1,"Reload")
	RIFG A 0 A_JumpIfInventory("FiredPrimary",1,"NoAmmo2")
	Goto Ready3
Reload:
	SHTZ A 0 
	A12G A 0 A_WeaponReady
	A12G A 0 A_ZoomFactor(1.0)
	A12G A 0 A_Takeinventory("Reloading",1)
	A12G A 0 A_Takeinventory("ADSmode",1)
	A12G A 0 A_Takeinventory("Zoomed",1)
	A12G A 0 A_JumpIfInventory("AssaultShotgunAmmo",20,"Ready")
	A12G A 0 A_JumpIfInventory("AmmoShell",1,3)
	Goto Ready
	TNT1 AAA 0 
	A12G A 0 A_Takeinventory("Zoomed",1)
	A12G A 0 A_GiveInventory ("Pumping", 1)
	A12G A 0 A_Takeinventory("Reloading",1)
	A12G A 0 A_JumpIfInventory("AutoShotUnloaded", 1, "PutMag")
	A12G A 0 A_JumpIfInventory("TurboReload", 1, "InsertBullets")
	A12G A 0 
	A12R A 1 Offset(-32, 42)
	A12R A 1 Offset(-24, 38)
	A12R A 1 Offset(-12, 34)
	A12R A 1 Offset(0, 32)
	A12R BCDEEE 1 A_WeaponReady(WRF_NOBOB | WRF_NOFIRE)
	A12R A 0 A_JumpIfInventory("Kicking",1,"DoKick")
	A12G A 0 A_PlaySound("AACL", 1)
	A12R FG 1 A_WeaponReady(WRF_NOBOB | WRF_NOFIRE)
	A12R G 1 Offset(3, 36)
	A12R GG 1 Offset(7, 38)
	A12R G 1 Offset(3, 36)
	A12R GGGG 1 Offset(0, 32)
	A12R A 0 A_JumpIfInventory("Kicking",1,"DoKick")
	A12R HIJKLLLLL 1 
	A12G A 0 A_PlaySound("Reload", 1)
	A12R MNOP 1 
	A12G A 0 A_Giveinventory("HasUnloaded",1)
	A12G A 0 A_GiveInventory("AutoShotUnloaded",1)
	A12G A 0 A_JumpIfInventory("realreloadOFF",1,2)
	A12G A 0 A_Takeinventory("AssaultShotgunAmmo",20)
PutMag:
	A12R Q 20 A_JumpIfInventory("Kicking",1,"DoKick")
	A12G A 0 A_Takeinventory("Reloading",1)
	A12G A 0 A_Takeinventory("ADSmode",1)
	A12G A 0 A_Takeinventory("Zoomed",1)
	A12G A 0 A_Takeinventory("HasUnloaded",1)
	A12G A 0 A_Takeinventory("AutoShotUnloaded",1)
	A12R RSTU 1 
	A12R LUUUU 1 
	A12R KJIHGGGG 1 
	A12G A 0 A_PlaySound("AACL", 4)
	A12R FEEEEEEEEDCBA 1 
	A12G A 0 A_Takeinventory("HasUnloaded",1)
	A12G A 0 A_Takeinventory("AutoShotUnloaded",1)
InsertBullets:
	TNT1 AAAA 0 
	A12G A 0 A_JumpIfInventory("AssaultShotgunAmmo",20,15)
	A12G A 0 A_JumpIfInventory("AmmoShell",1,3)
	Goto Ready
	TNT1 AAAAAA 0 
	A12G A 0 A_Giveinventory("AssaultShotgunAmmo",1)
	A12G A 0 A_Takeinventory("AmmoShell",1)
	Goto InsertBullets
	TNT1 AAAAAAAAAA 0 
	A12G A 0 A_Takeinventory("Reloading",1)
	A12G A 0 A_Refire
	Goto Ready
	TNT1 AAAA 0 
	A12G A 0 A_Takeinventory("Reloading",1)
	A12G A 0 A_Refire
	Goto Ready
InsertBullets2:
	TNT1 AAAA 0 
	A12G A 0 A_JumpIfInventory("AssaultShotgunAmmo",21,15)
	A12G A 0 A_JumpIfInventory("AmmoShell",1,3)
	Goto Ready
	TNT1 AAAAAA 0 
	A12G A 0 A_Takeinventory("HasUnloaded",1)
	A12G A 0 A_Takeinventory("AutoShotUnloaded",1)
	A12G A 0 A_Giveinventory("AssaultShotgunAmmo",1)
	A12G A 0 A_Takeinventory("AmmoShell",1)
	Goto InsertBullets2
	TNT1 AAAAAAAAAA 0 
	A12G A 0 A_Takeinventory("Reloading",1)
	A12G A 0 A_Refire
	Goto Ready
	TNT1 AAAA 0 
	A12G A 0 A_Takeinventory("Reloading",1)
	A12G A 0 A_Refire
	Goto Ready
Unload:
	A12G A 1 A_WeaponReady
	A12G A 0 A_Takeinventory("Unloading",1)
	A12G A 0 A_Takeinventory("ADSmode",1)
	A12G A 0 A_Takeinventory("Zoomed",1)
	A12G A 0 A_ZoomFactor(1.0)
	A12G A 0 A_JumpIfInventory("AssaultShotgunAmmo",1,3)
	Goto Ready
	TNT1 AAA 0 
	A12G A 0 A_GiveInventory ("Pumping", 1)
	A12G A 0 
	A12R A 1 Offset(-32, 42)
	A12R A 1 Offset(-24, 38)
	A12R A 1 Offset(-12, 34)
	A12R A 1 Offset(0, 32)
	A12R BCDEEE 1 A_WeaponReady(WRF_NOBOB | WRF_NOFIRE)
	A12G A 0 A_PlaySound("AACL", 1)
	A12R FG 1 A_WeaponReady(WRF_NOBOB | WRF_NOFIRE)
	A12R G 1 Offset(3, 36)
	A12R GG 1 Offset(7, 38)
	A12R G 1 Offset(3, 36)
	A12R GGGG 1 Offset(0, 32)
	A12R HIJKLLLLL 1 
	A12G A 0 A_PlaySound("Reload", 1)
	A12R MNOPQ 1 
	A12G A 0 A_TakeInventory ("Pumping", 1)
RemoveBullets:
	TNT1 AAAA 0 
	A12G A 0 A_JumpIfInventory("AssaultShotgunAmmo",1,3)
	Goto FinishUnload
	TNT1 AAAAAA 0 
	A12G A 0 A_Takeinventory("AssaultShotgunAmmo",1)
	A12G A 0 A_Giveinventory("AmmoShell",1)
	Goto RemoveBullets
FInishUnload:
	A12G A 0 
	A12G A 0 A_Giveinventory("HasUnloaded",1)
	A12G A 0 A_Giveinventory("AutoShotUnloaded",1)
	A12G A 0 A_Takeinventory("Unloading",1)
	Goto ReadyNoFuckingAmmo
Spawn:
	A12P A -1 
	Stop
`
