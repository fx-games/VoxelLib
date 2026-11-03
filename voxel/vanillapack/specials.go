package vanillapack

// specials are the blocks no family draws, in the reference's numbers
// (see special): their models and blockstate cases.
var specials = map[string]string{
	"activator_rail": `
	m $ rail_flat rail=block/$
	m $_on rail_flat rail=block/$_on
	m $_on_raised_ne template_rail_raised_ne rail=block/$_on
	m $_on_raised_sw template_rail_raised_sw rail=block/$_on
	m $_raised_ne template_rail_raised_ne rail=block/$
	m $_raised_sw template_rail_raised_sw rail=block/$
	v powered=false,shape=ascending_east -> $_raised_ne y90
	v powered=false,shape=ascending_north -> $_raised_ne
	v powered=false,shape=ascending_south -> $_raised_sw
	v powered=false,shape=ascending_west -> $_raised_sw y90
	v powered=false,shape=east_west -> $ y90
	v powered=false,shape=north_south -> $
	v powered=true,shape=ascending_east -> $_on_raised_ne y90
	v powered=true,shape=ascending_north -> $_on_raised_ne
	v powered=true,shape=ascending_south -> $_on_raised_sw
	v powered=true,shape=ascending_west -> $_on_raised_sw y90
	v powered=true,shape=east_west -> $_on y90
	v powered=true,shape=north_south -> $_on`,
	"amethyst_cluster": `
	m $ cross cross=block/$
	v facing=down -> $ x180
	v facing=east -> $ x90 y90
	v facing=north -> $ x90
	v facing=south -> $ x90 y180
	v facing=up -> $
	v facing=west -> $ x90 y270`,
	"anvil": `
	m $ template_$ top=block/$_top
	v facing=east -> $ y270
	v facing=north -> $ y180
	v facing=south -> $
	v facing=west -> $ y90`,
	"attached_melon_stem": `
	m $ stem_fruit stem=block/melon_stem upperstem=block/$
	v facing=east -> $ y180
	v facing=north -> $ y90
	v facing=south -> $ y270
	v facing=west -> $`,
	"attached_pumpkin_stem": `
	m $ stem_fruit stem=block/pumpkin_stem upperstem=block/$
	v facing=east -> $ y180
	v facing=north -> $ y90
	v facing=south -> $ y270
	v facing=west -> $`,
	"bamboo": `
	p {"age":"0"} -> $1_age0 | $2_age0 | $3_age0 | $4_age0
	p {"age":"1"} -> $1_age1 | $2_age1 | $3_age1 | $4_age1
	p {"leaves":"small"} -> $_small_leaves
	p {"leaves":"large"} -> $_large_leaves`,
	"bamboo_block": `
	m $_x cube_column_uv_locked_x end=block/$_top side=block/$
	m $_y cube_column_uv_locked_y end=block/$_top side=block/$
	m $_z cube_column_uv_locked_z end=block/$_top side=block/$
	v axis=x -> $_x
	v axis=y -> $_y
	v axis=z -> $_z`,
	"bamboo_fence": `
	m $_post custom_fence_post particle=block/$_particle texture=block/$
	m $_side_east custom_fence_side_east texture=block/$
	m $_side_north custom_fence_side_north texture=block/$
	m $_side_south custom_fence_side_south texture=block/$
	m $_side_west custom_fence_side_west texture=block/$
	p - -> $_post
	p {"north":"true"} -> $_side_north
	p {"east":"true"} -> $_side_east
	p {"south":"true"} -> $_side_south
	p {"west":"true"} -> $_side_west`,
	"bamboo_fence_gate": `
	m $ template_custom_fence_gate particle=block/$_particle texture=block/$
	m $_open template_custom_fence_gate_open particle=block/$_particle texture=block/$
	m $_wall template_custom_fence_gate_wall particle=block/$_particle texture=block/$
	m $_wall_open template_custom_fence_gate_wall_open particle=block/$_particle texture=block/$
	v facing=east,in_wall=false,open=false -> $ y270
	v facing=east,in_wall=false,open=true -> $_open y270
	v facing=east,in_wall=true,open=false -> $_wall y270
	v facing=east,in_wall=true,open=true -> $_wall_open y270
	v facing=north,in_wall=false,open=false -> $ y180
	v facing=north,in_wall=false,open=true -> $_open y180
	v facing=north,in_wall=true,open=false -> $_wall y180
	v facing=north,in_wall=true,open=true -> $_wall_open y180
	v facing=south,in_wall=false,open=false -> $
	v facing=south,in_wall=false,open=true -> $_open
	v facing=south,in_wall=true,open=false -> $_wall
	v facing=south,in_wall=true,open=true -> $_wall_open
	v facing=west,in_wall=false,open=false -> $ y90
	v facing=west,in_wall=false,open=true -> $_open y90
	v facing=west,in_wall=true,open=false -> $_wall y90
	v facing=west,in_wall=true,open=true -> $_wall_open y90`,
	"barrel": `
	m $ cube_bottom_top bottom=block/$_bottom side=block/$_side top=block/$_top
	m $_open cube_bottom_top bottom=block/$_bottom side=block/$_side top=block/$_top_open
	v facing=down,open=false -> $ x180
	v facing=down,open=true -> $_open x180
	v facing=east,open=false -> $ x90 y90
	v facing=east,open=true -> $_open x90 y90
	v facing=north,open=false -> $ x90
	v facing=north,open=true -> $_open x90
	v facing=south,open=false -> $ x90 y180
	v facing=south,open=true -> $_open x90 y180
	v facing=up,open=false -> $
	v facing=up,open=true -> $_open
	v facing=west,open=false -> $ x90 y270
	v facing=west,open=true -> $_open x90 y270`,
	"basalt": `
	m $ cube_column end=block/$_top side=block/$_side
	v axis=x -> $ x90 y90
	v axis=y -> $
	v axis=z -> $ x90`,
	"bedrock": `
	m $ cube_all all=block/$
	m $_mirrored cube_mirrored_all all=block/$
	v  -> $ | $_mirrored | $ y180 | $_mirrored y180`,
	"bee_nest": `
	m $_empty orientable_with_bottom bottom=block/$_bottom front=block/$_front particle=block/$_side side=block/$_side top=block/$_top
	m $_honey orientable_with_bottom bottom=block/$_bottom front=block/$_front_honey particle=block/$_side side=block/$_side top=block/$_top
	v facing=east,honey_level=0 -> $_empty y90
	v facing=east,honey_level=1 -> $_empty y90
	v facing=east,honey_level=2 -> $_empty y90
	v facing=east,honey_level=3 -> $_empty y90
	v facing=east,honey_level=4 -> $_empty y90
	v facing=east,honey_level=5 -> $_honey y90
	v facing=north,honey_level=0 -> $_empty
	v facing=north,honey_level=1 -> $_empty
	v facing=north,honey_level=2 -> $_empty
	v facing=north,honey_level=3 -> $_empty
	v facing=north,honey_level=4 -> $_empty
	v facing=north,honey_level=5 -> $_honey
	v facing=south,honey_level=0 -> $_empty y180
	v facing=south,honey_level=1 -> $_empty y180
	v facing=south,honey_level=2 -> $_empty y180
	v facing=south,honey_level=3 -> $_empty y180
	v facing=south,honey_level=4 -> $_empty y180
	v facing=south,honey_level=5 -> $_honey y180
	v facing=west,honey_level=0 -> $_empty y270
	v facing=west,honey_level=1 -> $_empty y270
	v facing=west,honey_level=2 -> $_empty y270
	v facing=west,honey_level=3 -> $_empty y270
	v facing=west,honey_level=4 -> $_empty y270
	v facing=west,honey_level=5 -> $_honey y270`,
	"beehive": `
	m $_empty orientable_with_bottom bottom=block/$_end front=block/$_front particle=block/$_side side=block/$_side top=block/$_end
	m $_honey orientable_with_bottom bottom=block/$_end front=block/$_front_honey particle=block/$_side side=block/$_side top=block/$_end
	v facing=east,honey_level=0 -> $_empty y90
	v facing=east,honey_level=1 -> $_empty y90
	v facing=east,honey_level=2 -> $_empty y90
	v facing=east,honey_level=3 -> $_empty y90
	v facing=east,honey_level=4 -> $_empty y90
	v facing=east,honey_level=5 -> $_honey y90
	v facing=north,honey_level=0 -> $_empty
	v facing=north,honey_level=1 -> $_empty
	v facing=north,honey_level=2 -> $_empty
	v facing=north,honey_level=3 -> $_empty
	v facing=north,honey_level=4 -> $_empty
	v facing=north,honey_level=5 -> $_honey
	v facing=south,honey_level=0 -> $_empty y180
	v facing=south,honey_level=1 -> $_empty y180
	v facing=south,honey_level=2 -> $_empty y180
	v facing=south,honey_level=3 -> $_empty y180
	v facing=south,honey_level=4 -> $_empty y180
	v facing=south,honey_level=5 -> $_honey y180
	v facing=west,honey_level=0 -> $_empty y270
	v facing=west,honey_level=1 -> $_empty y270
	v facing=west,honey_level=2 -> $_empty y270
	v facing=west,honey_level=3 -> $_empty y270
	v facing=west,honey_level=4 -> $_empty y270
	v facing=west,honey_level=5 -> $_honey y270`,
	"beetroots": `
	m $_stage0 crop crop=block/$_stage0
	m $_stage1 crop crop=block/$_stage1
	m $_stage2 crop crop=block/$_stage2
	m $_stage3 crop crop=block/$_stage3
	v age=0 -> $_stage0
	v age=1 -> $_stage1
	v age=2 -> $_stage2
	v age=3 -> $_stage3`,
	"bell": `
	v attachment=ceiling,facing=east -> $_ceiling y90
	v attachment=ceiling,facing=north -> $_ceiling
	v attachment=ceiling,facing=south -> $_ceiling y180
	v attachment=ceiling,facing=west -> $_ceiling y270
	v attachment=double_wall,facing=east -> $_between_walls
	v attachment=double_wall,facing=north -> $_between_walls y270
	v attachment=double_wall,facing=south -> $_between_walls y90
	v attachment=double_wall,facing=west -> $_between_walls y180
	v attachment=floor,facing=east -> $_floor y90
	v attachment=floor,facing=north -> $_floor
	v attachment=floor,facing=south -> $_floor y180
	v attachment=floor,facing=west -> $_floor y270
	v attachment=single_wall,facing=east -> $_wall
	v attachment=single_wall,facing=north -> $_wall y270
	v attachment=single_wall,facing=south -> $_wall y90
	v attachment=single_wall,facing=west -> $_wall y180`,
	"big_dripleaf": `
	v facing=east,tilt=full -> $_full_tilt y90
	v facing=east,tilt=none -> $ y90
	v facing=east,tilt=partial -> $_partial_tilt y90
	v facing=east,tilt=unstable -> $ y90
	v facing=north,tilt=full -> $_full_tilt
	v facing=north,tilt=none -> $
	v facing=north,tilt=partial -> $_partial_tilt
	v facing=north,tilt=unstable -> $
	v facing=south,tilt=full -> $_full_tilt y180
	v facing=south,tilt=none -> $ y180
	v facing=south,tilt=partial -> $_partial_tilt y180
	v facing=south,tilt=unstable -> $ y180
	v facing=west,tilt=full -> $_full_tilt y270
	v facing=west,tilt=none -> $ y270
	v facing=west,tilt=partial -> $_partial_tilt y270
	v facing=west,tilt=unstable -> $ y270`,
	"big_dripleaf_stem": `
	v facing=east -> $ y90
	v facing=north -> $
	v facing=south -> $ y180
	v facing=west -> $ y270`,
	"blast_furnace": `
	m $ orientable front=block/$_front side=block/$_side top=block/$_top
	m $_on orientable front=block/$_front_on side=block/$_side top=block/$_top
	v facing=east,lit=false -> $ y90
	v facing=east,lit=true -> $_on y90
	v facing=north,lit=false -> $
	v facing=north,lit=true -> $_on
	v facing=south,lit=false -> $ y180
	v facing=south,lit=true -> $_on y180
	v facing=west,lit=false -> $ y270
	v facing=west,lit=true -> $_on y270`,
	"bone_block": "like basalt",
	"brewing_stand": `
	p - -> $
	p {"has_bottle_0":"true"} -> $_bottle0
	p {"has_bottle_1":"true"} -> $_bottle1
	p {"has_bottle_2":"true"} -> $_bottle2
	p {"has_bottle_0":"false"} -> $_empty0
	p {"has_bottle_1":"false"} -> $_empty1
	p {"has_bottle_2":"false"} -> $_empty2`,
	"brown_mushroom_block": `
	m $ template_single_face texture=block/$
	m mushroom_block_inside template_single_face texture=block/mushroom_block_inside
	p {"north":"true"} -> $
	p {"east":"true"} -> $ y90 L
	p {"south":"true"} -> $ y180 L
	p {"west":"true"} -> $ y270 L
	p {"up":"true"} -> $ x270 L
	p {"down":"true"} -> $ x90 L
	p {"north":"false"} -> mushroom_block_inside
	p {"east":"false"} -> mushroom_block_inside y90
	p {"south":"false"} -> mushroom_block_inside y180
	p {"west":"false"} -> mushroom_block_inside y270
	p {"up":"false"} -> mushroom_block_inside x270
	p {"down":"false"} -> mushroom_block_inside x90`,
	"bubble_column": `
	m water - particle=block/water_still
	v  -> water`,
	"cake": `
	v bites=0 -> $
	v bites=1 -> $_slice1
	v bites=2 -> $_slice2
	v bites=3 -> $_slice3
	v bites=4 -> $_slice4
	v bites=5 -> $_slice5
	v bites=6 -> $_slice6`,
	"calibrated_sculk_sensor": `
	m $_active $ tendrils=block/sculk_sensor_tendril_active
	m $_inactive $ tendrils=block/sculk_sensor_tendril_inactive
	v facing=east,sculk_sensor_phase=active -> $_active y90
	v facing=east,sculk_sensor_phase=cooldown -> $_active y90
	v facing=east,sculk_sensor_phase=inactive -> $_inactive y90
	v facing=north,sculk_sensor_phase=active -> $_active
	v facing=north,sculk_sensor_phase=cooldown -> $_active
	v facing=north,sculk_sensor_phase=inactive -> $_inactive
	v facing=south,sculk_sensor_phase=active -> $_active y180
	v facing=south,sculk_sensor_phase=cooldown -> $_active y180
	v facing=south,sculk_sensor_phase=inactive -> $_inactive y180
	v facing=west,sculk_sensor_phase=active -> $_active y270
	v facing=west,sculk_sensor_phase=cooldown -> $_active y270
	v facing=west,sculk_sensor_phase=inactive -> $_inactive y270`,
	"campfire": `
	m $ template_$ fire=block/$_fire lit_log=block/$_log_lit
	v facing=east,lit=false -> $_off y270
	v facing=east,lit=true -> $ y270
	v facing=north,lit=false -> $_off y180
	v facing=north,lit=true -> $ y180
	v facing=south,lit=false -> $_off
	v facing=south,lit=true -> $
	v facing=west,lit=false -> $_off y90
	v facing=west,lit=true -> $ y90`,
	"carrots": `
	m $_stage0 crop crop=block/$_stage0
	m $_stage1 crop crop=block/$_stage1
	m $_stage2 crop crop=block/$_stage2
	m $_stage3 crop crop=block/$_stage3
	v age=0 -> $_stage0
	v age=1 -> $_stage0
	v age=2 -> $_stage1
	v age=3 -> $_stage1
	v age=4 -> $_stage2
	v age=5 -> $_stage2
	v age=6 -> $_stage2
	v age=7 -> $_stage3`,
	"carved_pumpkin": `
	m $ orientable front=block/$ side=block/pumpkin_side top=block/pumpkin_top
	v facing=east -> $ y90
	v facing=north -> $
	v facing=south -> $ y180
	v facing=west -> $ y270`,
	"cave_air": `
	m air - particle=missingno
	v  -> air`,
	"cave_vines": `
	m $ cross cross=block/$
	m $_lit cross cross=block/$_lit
	v berries=false -> $
	v berries=true -> $_lit`,
	"cave_vines_plant": "like cave_vines",
	"chain_command_block": `
	m $ template_command_block back=block/$_back front=block/$_front side=block/$_side
	m $_conditional template_command_block back=block/$_back front=block/$_front side=block/$_conditional
	v conditional=false,facing=down -> $ x90
	v conditional=false,facing=east -> $ y90
	v conditional=false,facing=north -> $
	v conditional=false,facing=south -> $ y180
	v conditional=false,facing=up -> $ x270
	v conditional=false,facing=west -> $ y270
	v conditional=true,facing=down -> $_conditional x90
	v conditional=true,facing=east -> $_conditional y90
	v conditional=true,facing=north -> $_conditional
	v conditional=true,facing=south -> $_conditional y180
	v conditional=true,facing=up -> $_conditional x270
	v conditional=true,facing=west -> $_conditional y270`,
	"cherry_log": "like bamboo_block",
	"chipped_anvil": `
	m $ template_anvil top=block/$_top
	v facing=east -> $ y270
	v facing=north -> $ y180
	v facing=south -> $
	v facing=west -> $ y90`,
	"chiseled_bookshelf": `
	m $_empty_slot_bottom_left template_$_slot_bottom_left texture=block/$_empty
	m $_empty_slot_bottom_mid template_$_slot_bottom_mid texture=block/$_empty
	m $_empty_slot_bottom_right template_$_slot_bottom_right texture=block/$_empty
	m $_empty_slot_top_left template_$_slot_top_left texture=block/$_empty
	m $_empty_slot_top_mid template_$_slot_top_mid texture=block/$_empty
	m $_empty_slot_top_right template_$_slot_top_right texture=block/$_empty
	m $_occupied_slot_bottom_left template_$_slot_bottom_left texture=block/$_occupied
	m $_occupied_slot_bottom_mid template_$_slot_bottom_mid texture=block/$_occupied
	m $_occupied_slot_bottom_right template_$_slot_bottom_right texture=block/$_occupied
	m $_occupied_slot_top_left template_$_slot_top_left texture=block/$_occupied
	m $_occupied_slot_top_mid template_$_slot_top_mid texture=block/$_occupied
	m $_occupied_slot_top_right template_$_slot_top_right texture=block/$_occupied
	p {"facing":"north"} -> $ L
	p {"AND":[{"facing":"north"},{"slot_0_occupied":"true"}]} -> $_occupied_slot_top_left
	p {"AND":[{"facing":"north"},{"slot_0_occupied":"false"}]} -> $_empty_slot_top_left
	p {"AND":[{"facing":"north"},{"slot_1_occupied":"true"}]} -> $_occupied_slot_top_mid
	p {"AND":[{"facing":"north"},{"slot_1_occupied":"false"}]} -> $_empty_slot_top_mid
	p {"AND":[{"facing":"north"},{"slot_2_occupied":"true"}]} -> $_occupied_slot_top_right
	p {"AND":[{"facing":"north"},{"slot_2_occupied":"false"}]} -> $_empty_slot_top_right
	p {"AND":[{"facing":"north"},{"slot_3_occupied":"true"}]} -> $_occupied_slot_bottom_left
	p {"AND":[{"facing":"north"},{"slot_3_occupied":"false"}]} -> $_empty_slot_bottom_left
	p {"AND":[{"facing":"north"},{"slot_4_occupied":"true"}]} -> $_occupied_slot_bottom_mid
	p {"AND":[{"facing":"north"},{"slot_4_occupied":"false"}]} -> $_empty_slot_bottom_mid
	p {"AND":[{"facing":"north"},{"slot_5_occupied":"true"}]} -> $_occupied_slot_bottom_right
	p {"AND":[{"facing":"north"},{"slot_5_occupied":"false"}]} -> $_empty_slot_bottom_right
	p {"facing":"east"} -> $ y90 L
	p {"AND":[{"facing":"east"},{"slot_0_occupied":"true"}]} -> $_occupied_slot_top_left y90
	p {"AND":[{"facing":"east"},{"slot_0_occupied":"false"}]} -> $_empty_slot_top_left y90
	p {"AND":[{"facing":"east"},{"slot_1_occupied":"true"}]} -> $_occupied_slot_top_mid y90
	p {"AND":[{"facing":"east"},{"slot_1_occupied":"false"}]} -> $_empty_slot_top_mid y90
	p {"AND":[{"facing":"east"},{"slot_2_occupied":"true"}]} -> $_occupied_slot_top_right y90
	p {"AND":[{"facing":"east"},{"slot_2_occupied":"false"}]} -> $_empty_slot_top_right y90
	p {"AND":[{"facing":"east"},{"slot_3_occupied":"true"}]} -> $_occupied_slot_bottom_left y90
	p {"AND":[{"facing":"east"},{"slot_3_occupied":"false"}]} -> $_empty_slot_bottom_left y90
	p {"AND":[{"facing":"east"},{"slot_4_occupied":"true"}]} -> $_occupied_slot_bottom_mid y90
	p {"AND":[{"facing":"east"},{"slot_4_occupied":"false"}]} -> $_empty_slot_bottom_mid y90
	p {"AND":[{"facing":"east"},{"slot_5_occupied":"true"}]} -> $_occupied_slot_bottom_right y90
	p {"AND":[{"facing":"east"},{"slot_5_occupied":"false"}]} -> $_empty_slot_bottom_right y90
	p {"facing":"south"} -> $ y180 L
	p {"AND":[{"facing":"south"},{"slot_0_occupied":"true"}]} -> $_occupied_slot_top_left y180
	p {"AND":[{"facing":"south"},{"slot_0_occupied":"false"}]} -> $_empty_slot_top_left y180
	p {"AND":[{"facing":"south"},{"slot_1_occupied":"true"}]} -> $_occupied_slot_top_mid y180
	p {"AND":[{"facing":"south"},{"slot_1_occupied":"false"}]} -> $_empty_slot_top_mid y180
	p {"AND":[{"facing":"south"},{"slot_2_occupied":"true"}]} -> $_occupied_slot_top_right y180
	p {"AND":[{"facing":"south"},{"slot_2_occupied":"false"}]} -> $_empty_slot_top_right y180
	p {"AND":[{"facing":"south"},{"slot_3_occupied":"true"}]} -> $_occupied_slot_bottom_left y180
	p {"AND":[{"facing":"south"},{"slot_3_occupied":"false"}]} -> $_empty_slot_bottom_left y180
	p {"AND":[{"facing":"south"},{"slot_4_occupied":"true"}]} -> $_occupied_slot_bottom_mid y180
	p {"AND":[{"facing":"south"},{"slot_4_occupied":"false"}]} -> $_empty_slot_bottom_mid y180
	p {"AND":[{"facing":"south"},{"slot_5_occupied":"true"}]} -> $_occupied_slot_bottom_right y180
	p {"AND":[{"facing":"south"},{"slot_5_occupied":"false"}]} -> $_empty_slot_bottom_right y180
	p {"facing":"west"} -> $ y270 L
	p {"AND":[{"facing":"west"},{"slot_0_occupied":"true"}]} -> $_occupied_slot_top_left y270
	p {"AND":[{"facing":"west"},{"slot_0_occupied":"false"}]} -> $_empty_slot_top_left y270
	p {"AND":[{"facing":"west"},{"slot_1_occupied":"true"}]} -> $_occupied_slot_top_mid y270
	p {"AND":[{"facing":"west"},{"slot_1_occupied":"false"}]} -> $_empty_slot_top_mid y270
	p {"AND":[{"facing":"west"},{"slot_2_occupied":"true"}]} -> $_occupied_slot_top_right y270
	p {"AND":[{"facing":"west"},{"slot_2_occupied":"false"}]} -> $_empty_slot_top_right y270
	p {"AND":[{"facing":"west"},{"slot_3_occupied":"true"}]} -> $_occupied_slot_bottom_left y270
	p {"AND":[{"facing":"west"},{"slot_3_occupied":"false"}]} -> $_empty_slot_bottom_left y270
	p {"AND":[{"facing":"west"},{"slot_4_occupied":"true"}]} -> $_occupied_slot_bottom_mid y270
	p {"AND":[{"facing":"west"},{"slot_4_occupied":"false"}]} -> $_empty_slot_bottom_mid y270
	p {"AND":[{"facing":"west"},{"slot_5_occupied":"true"}]} -> $_occupied_slot_bottom_right y270
	p {"AND":[{"facing":"west"},{"slot_5_occupied":"false"}]} -> $_empty_slot_bottom_right y270`,
	"chorus_flower": `
	m $ template_$ texture=block/$
	m $_dead template_$ texture=block/$_dead
	v age=0 -> $
	v age=1 -> $
	v age=2 -> $
	v age=3 -> $
	v age=4 -> $
	v age=5 -> $_dead`,
	"chorus_plant": `
	p {"north":"true"} -> $_side
	p {"east":"true"} -> $_side y90 L
	p {"south":"true"} -> $_side y180 L
	p {"west":"true"} -> $_side y270 L
	p {"up":"true"} -> $_side x270 L
	p {"down":"true"} -> $_side x90 L
	p {"north":"false"} -> $_noside w2 | $_noside1 | $_noside2 | $_noside3
	p {"east":"false"} -> $_noside1 y90 L | $_noside2 y90 L | $_noside3 y90 L | $_noside y90 L w2
	p {"south":"false"} -> $_noside2 y180 L | $_noside3 y180 L | $_noside y180 L w2 | $_noside1 y180 L
	p {"west":"false"} -> $_noside3 y270 L | $_noside y270 L w2 | $_noside1 y270 L | $_noside2 y270 L
	p {"up":"false"} -> $_noside x270 L w2 | $_noside3 x270 L | $_noside1 x270 L | $_noside2 x270 L
	p {"down":"false"} -> $_noside3 x90 L | $_noside2 x90 L | $_noside1 x90 L | $_noside x90 L w2`,
	"cocoa": `
	v age=0,facing=east -> $_stage0 y270
	v age=0,facing=north -> $_stage0 y180
	v age=0,facing=south -> $_stage0
	v age=0,facing=west -> $_stage0 y90
	v age=1,facing=east -> $_stage1 y270
	v age=1,facing=north -> $_stage1 y180
	v age=1,facing=south -> $_stage1
	v age=1,facing=west -> $_stage1 y90
	v age=2,facing=east -> $_stage2 y270
	v age=2,facing=north -> $_stage2 y180
	v age=2,facing=south -> $_stage2
	v age=2,facing=west -> $_stage2 y90`,
	"command_block": `
	m $ template_$ back=block/$_back front=block/$_front side=block/$_side
	m $_conditional template_$ back=block/$_back front=block/$_front side=block/$_conditional
	v conditional=false,facing=down -> $ x90
	v conditional=false,facing=east -> $ y90
	v conditional=false,facing=north -> $
	v conditional=false,facing=south -> $ y180
	v conditional=false,facing=up -> $ x270
	v conditional=false,facing=west -> $ y270
	v conditional=true,facing=down -> $_conditional x90
	v conditional=true,facing=east -> $_conditional y90
	v conditional=true,facing=north -> $_conditional
	v conditional=true,facing=south -> $_conditional y180
	v conditional=true,facing=up -> $_conditional x270
	v conditional=true,facing=west -> $_conditional y270`,
	"comparator": `
	v facing=east,mode=compare,powered=false -> $ y270
	v facing=east,mode=compare,powered=true -> $_on y270
	v facing=east,mode=subtract,powered=false -> $_subtract y270
	v facing=east,mode=subtract,powered=true -> $_on_subtract y270
	v facing=north,mode=compare,powered=false -> $ y180
	v facing=north,mode=compare,powered=true -> $_on y180
	v facing=north,mode=subtract,powered=false -> $_subtract y180
	v facing=north,mode=subtract,powered=true -> $_on_subtract y180
	v facing=south,mode=compare,powered=false -> $
	v facing=south,mode=compare,powered=true -> $_on
	v facing=south,mode=subtract,powered=false -> $_subtract
	v facing=south,mode=subtract,powered=true -> $_on_subtract
	v facing=west,mode=compare,powered=false -> $ y90
	v facing=west,mode=compare,powered=true -> $_on y90
	v facing=west,mode=subtract,powered=false -> $_subtract y90
	v facing=west,mode=subtract,powered=true -> $_on_subtract y90`,
	"composter": `
	p - -> $
	p {"level":"1"} -> $_contents1
	p {"level":"2"} -> $_contents2
	p {"level":"3"} -> $_contents3
	p {"level":"4"} -> $_contents4
	p {"level":"5"} -> $_contents5
	p {"level":"6"} -> $_contents6
	p {"level":"7"} -> $_contents7
	p {"level":"8"} -> $_contents_ready`,
	"copper_bulb": `
	m $ cube_all all=block/$
	m $_lit cube_all all=block/$_lit
	m $_lit_powered cube_all all=block/$_lit_powered
	m $_powered cube_all all=block/$_powered
	v lit=false,powered=false -> $
	v lit=false,powered=true -> $_powered
	v lit=true,powered=false -> $_lit
	v lit=true,powered=true -> $_lit_powered`,
	"copper_chain": `
	m $ template_chain texture=block/$
	v axis=x -> $ x90 y90
	v axis=y -> $
	v axis=z -> $ x90`,
	"crafter": `
	m $_crafting_triggered $_crafting
	v crafting=false,orientation=down_east,triggered=false -> $ x90 y90
	v crafting=false,orientation=down_east,triggered=true -> $_triggered x90 y90
	v crafting=false,orientation=down_north,triggered=false -> $ x90
	v crafting=false,orientation=down_north,triggered=true -> $_triggered x90
	v crafting=false,orientation=down_south,triggered=false -> $ x90 y180
	v crafting=false,orientation=down_south,triggered=true -> $_triggered x90 y180
	v crafting=false,orientation=down_west,triggered=false -> $ x90 y270
	v crafting=false,orientation=down_west,triggered=true -> $_triggered x90 y270
	v crafting=false,orientation=east_up,triggered=false -> $ y90
	v crafting=false,orientation=east_up,triggered=true -> $_triggered y90
	v crafting=false,orientation=north_up,triggered=false -> $
	v crafting=false,orientation=north_up,triggered=true -> $_triggered
	v crafting=false,orientation=south_up,triggered=false -> $ y180
	v crafting=false,orientation=south_up,triggered=true -> $_triggered y180
	v crafting=false,orientation=up_east,triggered=false -> $ x270 y270
	v crafting=false,orientation=up_east,triggered=true -> $_triggered x270 y270
	v crafting=false,orientation=up_north,triggered=false -> $ x270 y180
	v crafting=false,orientation=up_north,triggered=true -> $_triggered x270 y180
	v crafting=false,orientation=up_south,triggered=false -> $ x270
	v crafting=false,orientation=up_south,triggered=true -> $_triggered x270
	v crafting=false,orientation=up_west,triggered=false -> $ x270 y90
	v crafting=false,orientation=up_west,triggered=true -> $_triggered x270 y90
	v crafting=false,orientation=west_up,triggered=false -> $ y270
	v crafting=false,orientation=west_up,triggered=true -> $_triggered y270
	v crafting=true,orientation=down_east,triggered=false -> $_crafting x90 y90
	v crafting=true,orientation=down_east,triggered=true -> $_crafting_triggered x90 y90
	v crafting=true,orientation=down_north,triggered=false -> $_crafting x90
	v crafting=true,orientation=down_north,triggered=true -> $_crafting_triggered x90
	v crafting=true,orientation=down_south,triggered=false -> $_crafting x90 y180
	v crafting=true,orientation=down_south,triggered=true -> $_crafting_triggered x90 y180
	v crafting=true,orientation=down_west,triggered=false -> $_crafting x90 y270
	v crafting=true,orientation=down_west,triggered=true -> $_crafting_triggered x90 y270
	v crafting=true,orientation=east_up,triggered=false -> $_crafting y90
	v crafting=true,orientation=east_up,triggered=true -> $_crafting_triggered y90
	v crafting=true,orientation=north_up,triggered=false -> $_crafting
	v crafting=true,orientation=north_up,triggered=true -> $_crafting_triggered
	v crafting=true,orientation=south_up,triggered=false -> $_crafting y180
	v crafting=true,orientation=south_up,triggered=true -> $_crafting_triggered y180
	v crafting=true,orientation=up_east,triggered=false -> $_crafting x270 y270
	v crafting=true,orientation=up_east,triggered=true -> $_crafting_triggered x270 y270
	v crafting=true,orientation=up_north,triggered=false -> $_crafting x270 y180
	v crafting=true,orientation=up_north,triggered=true -> $_crafting_triggered x270 y180
	v crafting=true,orientation=up_south,triggered=false -> $_crafting x270
	v crafting=true,orientation=up_south,triggered=true -> $_crafting_triggered x270
	v crafting=true,orientation=up_west,triggered=false -> $_crafting x270 y90
	v crafting=true,orientation=up_west,triggered=true -> $_crafting_triggered x270 y90
	v crafting=true,orientation=west_up,triggered=false -> $_crafting y270
	v crafting=true,orientation=west_up,triggered=true -> $_crafting_triggered y270`,
	"creaking_heart": `
	m $ cube_column end=block/$_top side=block/$
	m $_awake cube_column end=block/$_top_awake side=block/$_awake
	m $_awake_horizontal cube_column_horizontal end=block/$_top_awake side=block/$_awake
	m $_dormant cube_column end=block/$_top_dormant side=block/$_dormant
	m $_dormant_horizontal cube_column_horizontal end=block/$_top_dormant side=block/$_dormant
	m $_horizontal cube_column_horizontal end=block/$_top side=block/$
	v axis=x,creaking_heart_state=awake -> $_awake_horizontal x90 y90
	v axis=x,creaking_heart_state=dormant -> $_dormant_horizontal x90 y90
	v axis=x,creaking_heart_state=uprooted -> $_horizontal x90 y90
	v axis=y,creaking_heart_state=awake -> $_awake
	v axis=y,creaking_heart_state=dormant -> $_dormant
	v axis=y,creaking_heart_state=uprooted -> $
	v axis=z,creaking_heart_state=awake -> $_awake_horizontal x90
	v axis=z,creaking_heart_state=dormant -> $_dormant_horizontal x90
	v axis=z,creaking_heart_state=uprooted -> $_horizontal x90`,
	"crimson_stem": `
	m $ cube_column end=block/$_top side=block/$
	v axis=x -> $ x90 y90
	v axis=y -> $
	v axis=z -> $ x90`,
	"damaged_anvil": "like chipped_anvil",
	"daylight_detector": `
	m $ template_$ side=block/$_side top=block/$_top
	m $_inverted template_$ side=block/$_side top=block/$_inverted_top
	v inverted=false -> $
	v inverted=true -> $_inverted`,
	"deepslate": `
	m $ cube_column end=block/$_top side=block/$
	m $_mirrored cube_column_mirrored end=block/$_top side=block/$
	v axis=x -> $ x90 y90 | $_mirrored x90 y90 | $ x90 y90 | $_mirrored x90 y90
	v axis=y -> $ | $_mirrored | $ y180 | $_mirrored y180
	v axis=z -> $ x90 | $_mirrored x90 | $ x90 y180 | $_mirrored x90 y180`,
	"detector_rail": "like activator_rail",
	"dirt": `
	m $ cube_all all=block/$
	v  -> $ | $ y90 | $ y180 | $ y270`,
	"dirt_path": `
	v  -> $ | $ y90 | $ y180 | $ y270`,
	"dispenser": `
	m $ orientable front=block/$_front side=block/furnace_side top=block/furnace_top
	m $_vertical orientable_vertical front=block/$_front_vertical side=block/furnace_top
	v facing=down -> $_vertical x180
	v facing=east -> $ y90
	v facing=north -> $
	v facing=south -> $ y180
	v facing=up -> $_vertical
	v facing=west -> $ y270`,
	"dried_ghast": `
	m $_hydration_0 $ bottom=block/$_hydration_0_bottom east=block/$_hydration_0_east north=block/$_hydration_0_north particle=block/$_hydration_0_north south=block/$_hydration_0_south tentacles=block/$_hydration_0_tentacles top=block/$_hydration_0_top west=block/$_hydration_0_west
	m $_hydration_1 $ bottom=block/$_hydration_1_bottom east=block/$_hydration_1_east north=block/$_hydration_1_north particle=block/$_hydration_1_north south=block/$_hydration_1_south tentacles=block/$_hydration_1_tentacles top=block/$_hydration_1_top west=block/$_hydration_1_west
	m $_hydration_2 $ bottom=block/$_hydration_2_bottom east=block/$_hydration_2_east north=block/$_hydration_2_north particle=block/$_hydration_2_north south=block/$_hydration_2_south tentacles=block/$_hydration_2_tentacles top=block/$_hydration_2_top west=block/$_hydration_2_west
	m $_hydration_3 $ bottom=block/$_hydration_3_bottom east=block/$_hydration_3_east north=block/$_hydration_3_north particle=block/$_hydration_3_north south=block/$_hydration_3_south tentacles=block/$_hydration_3_tentacles top=block/$_hydration_3_top west=block/$_hydration_3_west
	v facing=east,hydration=0 -> $_hydration_0 y90
	v facing=east,hydration=1 -> $_hydration_1 y90
	v facing=east,hydration=2 -> $_hydration_2 y90
	v facing=east,hydration=3 -> $_hydration_3 y90
	v facing=north,hydration=0 -> $_hydration_0
	v facing=north,hydration=1 -> $_hydration_1
	v facing=north,hydration=2 -> $_hydration_2
	v facing=north,hydration=3 -> $_hydration_3
	v facing=south,hydration=0 -> $_hydration_0 y180
	v facing=south,hydration=1 -> $_hydration_1 y180
	v facing=south,hydration=2 -> $_hydration_2 y180
	v facing=south,hydration=3 -> $_hydration_3 y180
	v facing=west,hydration=0 -> $_hydration_0 y270
	v facing=west,hydration=1 -> $_hydration_1 y270
	v facing=west,hydration=2 -> $_hydration_2 y270
	v facing=west,hydration=3 -> $_hydration_3 y270`,
	"dropper": "like dispenser",
	"end_portal_frame": `
	v eye=false,facing=east -> $ y270
	v eye=false,facing=north -> $ y180
	v eye=false,facing=south -> $
	v eye=false,facing=west -> $ y90
	v eye=true,facing=east -> $_filled y270
	v eye=true,facing=north -> $_filled y180
	v eye=true,facing=south -> $_filled
	v eye=true,facing=west -> $_filled y90`,
	"end_rod": `
	v facing=down -> $ x180
	v facing=east -> $ x90 y90
	v facing=north -> $ x90
	v facing=south -> $ x90 y180
	v facing=up -> $
	v facing=west -> $ x90 y270`,
	"exposed_copper_bulb":  "like copper_bulb",
	"exposed_copper_chain": "like copper_chain",
	"exposed_lightning_rod": `
	m $ template_lightning_rod texture=block/$
	m lightning_rod_on template_lightning_rod texture=block/lightning_rod_on
	v facing=down,powered=false -> $ x180
	v facing=down,powered=true -> lightning_rod_on x180
	v facing=east,powered=false -> $ x90 y90
	v facing=east,powered=true -> lightning_rod_on x90 y90
	v facing=north,powered=false -> $ x90
	v facing=north,powered=true -> lightning_rod_on x90
	v facing=south,powered=false -> $ x90 y180
	v facing=south,powered=true -> lightning_rod_on x90 y180
	v facing=up,powered=false -> $
	v facing=up,powered=true -> lightning_rod_on
	v facing=west,powered=false -> $ x90 y270
	v facing=west,powered=true -> lightning_rod_on x90 y270`,
	"farmland": `
	m $ template_$ dirt=block/dirt top=block/$
	m $_moist template_$ dirt=block/dirt top=block/$_moist
	v moisture=0 -> $
	v moisture=1 -> $
	v moisture=2 -> $
	v moisture=3 -> $
	v moisture=4 -> $
	v moisture=5 -> $
	v moisture=6 -> $
	v moisture=7 -> $_moist`,
	"fire": `
	m $_floor0 template_$_floor $=block/$_0
	m $_floor1 template_$_floor $=block/$_1
	m $_side0 template_$_side $=block/$_0
	m $_side1 template_$_side $=block/$_1
	m $_side_alt0 template_$_side_alt $=block/$_0
	m $_side_alt1 template_$_side_alt $=block/$_1
	m $_up0 template_$_up $=block/$_0
	m $_up1 template_$_up $=block/$_1
	m $_up_alt0 template_$_up_alt $=block/$_0
	m $_up_alt1 template_$_up_alt $=block/$_1
	p {"east":"false","north":"false","south":"false","up":"false","west":"false"} -> $_floor0 | $_floor1
	p {"OR":[{"north":"true"},{"east":"false","north":"false","south":"false","up":"false","west":"false"}]} -> $_side0 | $_side1 | $_side_alt0 | $_side_alt1
	p {"OR":[{"east":"true"},{"east":"false","north":"false","south":"false","up":"false","west":"false"}]} -> $_side0 y90 | $_side1 y90 | $_side_alt0 y90 | $_side_alt1 y90
	p {"OR":[{"south":"true"},{"east":"false","north":"false","south":"false","up":"false","west":"false"}]} -> $_side0 y180 | $_side1 y180 | $_side_alt0 y180 | $_side_alt1 y180
	p {"OR":[{"west":"true"},{"east":"false","north":"false","south":"false","up":"false","west":"false"}]} -> $_side0 y270 | $_side1 y270 | $_side_alt0 y270 | $_side_alt1 y270
	p {"up":"true"} -> $_up0 | $_up1 | $_up_alt0 | $_up_alt1`,
	"frosted_ice": `
	m $_0 cube_all all=block/$_0
	m $_1 cube_all all=block/$_1
	m $_2 cube_all all=block/$_2
	m $_3 cube_all all=block/$_3
	v age=0 -> $_0
	v age=1 -> $_1
	v age=2 -> $_2
	v age=3 -> $_3`,
	"furnace": "like blast_furnace",
	"glow_item_frame": `
	m $ template_item_frame back=block/$ particle=block/birch_planks wood=block/birch_planks
	m $_map template_item_frame_map back=block/$ particle=block/birch_planks wood=block/birch_planks
	v map=false -> $
	v map=true -> $_map`,
	"glow_lichen": `
	p {"north":"true"} -> $
	p {"down":"false","east":"false","north":"false","south":"false","up":"false","west":"false"} -> $
	p {"east":"true"} -> $ y90 L
	p {"down":"false","east":"false","north":"false","south":"false","up":"false","west":"false"} -> $ y90 L
	p {"south":"true"} -> $ y180 L
	p {"down":"false","east":"false","north":"false","south":"false","up":"false","west":"false"} -> $ y180 L
	p {"west":"true"} -> $ y270 L
	p {"down":"false","east":"false","north":"false","south":"false","up":"false","west":"false"} -> $ y270 L
	p {"up":"true"} -> $ x270 L
	p {"down":"false","east":"false","north":"false","south":"false","up":"false","west":"false"} -> $ x270 L
	p {"down":"true"} -> $ x90 L
	p {"down":"false","east":"false","north":"false","south":"false","up":"false","west":"false"} -> $ x90 L`,
	"grass_block": `
	m $_snow cube_bottom_top bottom=block/dirt particle=block/dirt side=block/$_snow top=block/$_top
	v snowy=false -> $ | $ y90 | $ y180 | $ y270
	v snowy=true -> $_snow`,
	"grindstone": `
	v face=ceiling,facing=east -> $ x180 y270
	v face=ceiling,facing=north -> $ x180 y180
	v face=ceiling,facing=south -> $ x180
	v face=ceiling,facing=west -> $ x180 y90
	v face=floor,facing=east -> $ y90
	v face=floor,facing=north -> $
	v face=floor,facing=south -> $ y180
	v face=floor,facing=west -> $ y270
	v face=wall,facing=east -> $ x90 y90
	v face=wall,facing=north -> $ x90
	v face=wall,facing=south -> $ x90 y180
	v face=wall,facing=west -> $ x90 y270`,
	"hay_block": `
	m $ cube_column end=block/$_top side=block/$_side
	m $_horizontal cube_column_horizontal end=block/$_top side=block/$_side
	v axis=x -> $_horizontal x90 y90
	v axis=y -> $
	v axis=z -> $_horizontal x90`,
	"hopper": `
	v facing=down -> $
	v facing=east -> $_side y90
	v facing=north -> $_side
	v facing=south -> $_side y180
	v facing=west -> $_side y270`,
	"infested_deepslate": `
	m deepslate cube_column end=block/deepslate_top side=block/deepslate
	m deepslate_mirrored cube_column_mirrored end=block/deepslate_top side=block/deepslate
	v axis=x -> deepslate x90 y90 | deepslate_mirrored x90 y90 | deepslate x90 y90 | deepslate_mirrored x90 y90
	v axis=y -> deepslate | deepslate_mirrored | deepslate y180 | deepslate_mirrored y180
	v axis=z -> deepslate x90 | deepslate_mirrored x90 | deepslate x90 y180 | deepslate_mirrored x90 y180`,
	"infested_stone": `
	m stone cube_all all=block/stone
	m stone_mirrored cube_mirrored_all all=block/stone
	v  -> stone | stone_mirrored | stone y180 | stone_mirrored y180`,
	"iron_chain": "like copper_chain",
	"item_frame": `
	m $ template_$ back=block/$ particle=block/birch_planks wood=block/birch_planks
	m $_map template_$_map back=block/$ particle=block/birch_planks wood=block/birch_planks
	v map=false -> $
	v map=true -> $_map`,
	"jack_o_lantern": "like carved_pumpkin",
	"jigsaw": `
	m $ cube_directional down=block/$_side east=block/$_side north=block/$_top particle=block/$_top south=block/$_bottom up=block/$_lock west=block/$_side
	v orientation=down_east -> $ x90 y90
	v orientation=down_north -> $ x90
	v orientation=down_south -> $ x90 y180
	v orientation=down_west -> $ x90 y270
	v orientation=east_up -> $ y90
	v orientation=north_up -> $
	v orientation=south_up -> $ y180
	v orientation=up_east -> $ x270 y270
	v orientation=up_north -> $ x270 y180
	v orientation=up_south -> $ x270
	v orientation=up_west -> $ x270 y90
	v orientation=west_up -> $ y270`,
	"ladder":             "like big_dripleaf_stem",
	"large_amethyst_bud": "like amethyst_cluster",
	"large_fern": `
	m $_bottom tinted_cross cross=block/$_bottom
	m $_top tinted_cross cross=block/$_top
	v half=lower -> $_bottom
	v half=upper -> $_top`,
	"leaf_litter": `
	m $_1 template_$_1 texture=block/$
	m $_2 template_$_2 texture=block/$
	m $_3 template_$_3 texture=block/$
	m $_4 template_$_4 texture=block/$
	p {"facing":"north","segment_amount":"1"} -> $_1
	p {"facing":"east","segment_amount":"1"} -> $_1 y90
	p {"facing":"south","segment_amount":"1"} -> $_1 y180
	p {"facing":"west","segment_amount":"1"} -> $_1 y270
	p {"facing":"north","segment_amount":"2|3"} -> $_2
	p {"facing":"east","segment_amount":"2|3"} -> $_2 y90
	p {"facing":"south","segment_amount":"2|3"} -> $_2 y180
	p {"facing":"west","segment_amount":"2|3"} -> $_2 y270
	p {"facing":"north","segment_amount":"3"} -> $_3
	p {"facing":"east","segment_amount":"3"} -> $_3 y90
	p {"facing":"south","segment_amount":"3"} -> $_3 y180
	p {"facing":"west","segment_amount":"3"} -> $_3 y270
	p {"facing":"north","segment_amount":"4"} -> $_4
	p {"facing":"east","segment_amount":"4"} -> $_4 y90
	p {"facing":"south","segment_amount":"4"} -> $_4 y180
	p {"facing":"west","segment_amount":"4"} -> $_4 y270`,
	"lectern": "like big_dripleaf_stem",
	"lever": `
	v face=ceiling,facing=east,powered=false -> $_on x180 y270
	v face=ceiling,facing=east,powered=true -> $ x180 y270
	v face=ceiling,facing=north,powered=false -> $_on x180 y180
	v face=ceiling,facing=north,powered=true -> $ x180 y180
	v face=ceiling,facing=south,powered=false -> $_on x180
	v face=ceiling,facing=south,powered=true -> $ x180
	v face=ceiling,facing=west,powered=false -> $_on x180 y90
	v face=ceiling,facing=west,powered=true -> $ x180 y90
	v face=floor,facing=east,powered=false -> $_on y90
	v face=floor,facing=east,powered=true -> $ y90
	v face=floor,facing=north,powered=false -> $_on
	v face=floor,facing=north,powered=true -> $
	v face=floor,facing=south,powered=false -> $_on y180
	v face=floor,facing=south,powered=true -> $ y180
	v face=floor,facing=west,powered=false -> $_on y270
	v face=floor,facing=west,powered=true -> $ y270
	v face=wall,facing=east,powered=false -> $_on x90 y90
	v face=wall,facing=east,powered=true -> $ x90 y90
	v face=wall,facing=north,powered=false -> $_on x90
	v face=wall,facing=north,powered=true -> $ x90
	v face=wall,facing=south,powered=false -> $_on x90 y180
	v face=wall,facing=south,powered=true -> $ x90 y180
	v face=wall,facing=west,powered=false -> $_on x90 y270
	v face=wall,facing=west,powered=true -> $ x90 y270`,
	"light": `
	m $_00 - particle=item/$_00
	m $_01 - particle=item/$_01
	m $_02 - particle=item/$_02
	m $_03 - particle=item/$_03
	m $_04 - particle=item/$_04
	m $_05 - particle=item/$_05
	m $_06 - particle=item/$_06
	m $_07 - particle=item/$_07
	m $_08 - particle=item/$_08
	m $_09 - particle=item/$_09
	m $_10 - particle=item/$_10
	m $_11 - particle=item/$_11
	m $_12 - particle=item/$_12
	m $_13 - particle=item/$_13
	m $_14 - particle=item/$_14
	m $_15 - particle=item/$_15
	v level=0 -> $_00
	v level=1 -> $_01
	v level=10 -> $_10
	v level=11 -> $_11
	v level=12 -> $_12
	v level=13 -> $_13
	v level=14 -> $_14
	v level=15 -> $_15
	v level=2 -> $_02
	v level=3 -> $_03
	v level=4 -> $_04
	v level=5 -> $_05
	v level=6 -> $_06
	v level=7 -> $_07
	v level=8 -> $_08
	v level=9 -> $_09`,
	"lightning_rod": `
	m $ template_$ texture=block/$
	m $_on template_$ texture=block/$_on
	v facing=down,powered=false -> $ x180
	v facing=down,powered=true -> $_on x180
	v facing=east,powered=false -> $ x90 y90
	v facing=east,powered=true -> $_on x90 y90
	v facing=north,powered=false -> $ x90
	v facing=north,powered=true -> $_on x90
	v facing=south,powered=false -> $ x90 y180
	v facing=south,powered=true -> $_on x90 y180
	v facing=up,powered=false -> $
	v facing=up,powered=true -> $_on
	v facing=west,powered=false -> $ x90 y270
	v facing=west,powered=true -> $_on x90 y270`,
	"lilac": `
	m $_bottom cross cross=block/$_bottom
	m $_top cross cross=block/$_top
	v half=lower -> $_bottom
	v half=upper -> $_top`,
	"lily_pad": "like dirt_path",
	"loom": `
	m $ orientable_with_bottom bottom=block/$_bottom front=block/$_front side=block/$_side top=block/$_top
	v facing=east -> $ y90
	v facing=north -> $
	v facing=south -> $ y180
	v facing=west -> $ y270`,
	"mangrove_propagule": `
	v age=0,hanging=false -> $
	v age=0,hanging=true -> $_hanging_0
	v age=1,hanging=false -> $
	v age=1,hanging=true -> $_hanging_1
	v age=2,hanging=false -> $
	v age=2,hanging=true -> $_hanging_2
	v age=3,hanging=false -> $
	v age=3,hanging=true -> $_hanging_3
	v age=4,hanging=false -> $
	v age=4,hanging=true -> $_hanging_4`,
	"medium_amethyst_bud": "like amethyst_cluster",
	"melon_stem": `
	m $_stage0 stem_growth0 stem=block/$
	m $_stage1 stem_growth1 stem=block/$
	m $_stage2 stem_growth2 stem=block/$
	m $_stage3 stem_growth3 stem=block/$
	m $_stage4 stem_growth4 stem=block/$
	m $_stage5 stem_growth5 stem=block/$
	m $_stage6 stem_growth6 stem=block/$
	m $_stage7 stem_growth7 stem=block/$
	v age=0 -> $_stage0
	v age=1 -> $_stage1
	v age=2 -> $_stage2
	v age=3 -> $_stage3
	v age=4 -> $_stage4
	v age=5 -> $_stage5
	v age=6 -> $_stage6
	v age=7 -> $_stage7`,
	"mud_bricks": `
	m $ cube_all all=block/$
	m $_north_west_mirrored cube_north_west_mirrored_all all=block/$
	v  -> $_north_west_mirrored`,
	"muddy_mangrove_roots": "like basalt",
	"mushroom_stem": `
	m mushroom_block_inside template_single_face texture=block/mushroom_block_inside
	m $ template_single_face texture=block/$
	p {"north":"true"} -> $
	p {"east":"true"} -> $ y90 L
	p {"south":"true"} -> $ y180 L
	p {"west":"true"} -> $ y270 L
	p {"up":"true"} -> $ x270 L
	p {"down":"true"} -> $ x90 L
	p {"north":"false"} -> mushroom_block_inside
	p {"east":"false"} -> mushroom_block_inside y90
	p {"south":"false"} -> mushroom_block_inside y180
	p {"west":"false"} -> mushroom_block_inside y270
	p {"up":"false"} -> mushroom_block_inside x270
	p {"down":"false"} -> mushroom_block_inside x90`,
	"mycelium": `
	m grass_block_snow cube_bottom_top bottom=block/dirt particle=block/dirt side=block/grass_block_snow top=block/grass_block_top
	m $ cube_bottom_top bottom=block/dirt side=block/$_side top=block/$_top
	v snowy=false -> $ | $ y90 | $ y180 | $ y270
	v snowy=true -> grass_block_snow`,
	"nether_portal": `
	v axis=x -> $_ns
	v axis=z -> $_ew`,
	"nether_wart": `
	m $_stage0 crop crop=block/$_stage0
	m $_stage1 crop crop=block/$_stage1
	m $_stage2 crop crop=block/$_stage2
	v age=0 -> $_stage0
	v age=1 -> $_stage1
	v age=2 -> $_stage1
	v age=3 -> $_stage2`,
	"netherrack": `
	m $ cube_all all=block/$
	v  -> $ | $ x90 | $ x180 | $ x270 | $ y90 | $ x90 y90 | $ x180 y90 | $ x270 y90 | $ y180 | $ x90 y180 | $ x180 y180 | $ x270 y180 | $ y270 | $ x90 y270 | $ x180 y270 | $ x270 y270`,
	"observer": `
	m $_on $ bottom=block/$_back_on
	v facing=down,powered=false -> $ x90
	v facing=down,powered=true -> $_on x90
	v facing=east,powered=false -> $ y90
	v facing=east,powered=true -> $_on y90
	v facing=north,powered=false -> $
	v facing=north,powered=true -> $_on
	v facing=south,powered=false -> $ y180
	v facing=south,powered=true -> $_on y180
	v facing=up,powered=false -> $ x270
	v facing=up,powered=true -> $_on x270
	v facing=west,powered=false -> $ y270
	v facing=west,powered=true -> $_on y270`,
	"ochre_froglight":       "like hay_block",
	"oxidized_copper_bulb":  "like copper_bulb",
	"oxidized_copper_chain": "like copper_chain",
	"oxidized_lightning_rod": `
	m lightning_rod_on template_lightning_rod texture=block/lightning_rod_on
	m $ template_lightning_rod texture=block/$
	v facing=down,powered=false -> $ x180
	v facing=down,powered=true -> lightning_rod_on x180
	v facing=east,powered=false -> $ x90 y90
	v facing=east,powered=true -> lightning_rod_on x90 y90
	v facing=north,powered=false -> $ x90
	v facing=north,powered=true -> lightning_rod_on x90
	v facing=south,powered=false -> $ x90 y180
	v facing=south,powered=true -> lightning_rod_on x90 y180
	v facing=up,powered=false -> $
	v facing=up,powered=true -> lightning_rod_on
	v facing=west,powered=false -> $ x90 y270
	v facing=west,powered=true -> lightning_rod_on x90 y270`,
	"pale_hanging_moss": `
	m $ cross cross=block/$
	m $_tip cross cross=block/$_tip
	v tip=false -> $
	v tip=true -> $_tip`,
	"pale_moss_carpet": `
	m $ carpet wool=block/$
	m $_side_small mossy_carpet_side side=block/$_side_small
	m $_side_tall mossy_carpet_side side=block/$_side_tall
	p {"bottom":"true"} -> $
	p {"bottom":"false","east":"none","north":"none","south":"none","west":"none"} -> $
	p {"north":"tall"} -> $_side_tall
	p {"north":"low"} -> $_side_small
	p {"bottom":"false","east":"none","north":"none","south":"none","west":"none"} -> $_side_tall
	p {"east":"tall"} -> $_side_tall y90 L
	p {"east":"low"} -> $_side_small y90 L
	p {"bottom":"false","east":"none","north":"none","south":"none","west":"none"} -> $_side_tall y90 L
	p {"south":"tall"} -> $_side_tall y180 L
	p {"south":"low"} -> $_side_small y180 L
	p {"bottom":"false","east":"none","north":"none","south":"none","west":"none"} -> $_side_tall y180 L
	p {"west":"tall"} -> $_side_tall y270 L
	p {"west":"low"} -> $_side_small y270 L
	p {"bottom":"false","east":"none","north":"none","south":"none","west":"none"} -> $_side_tall y270 L`,
	"pearlescent_froglight": "like hay_block",
	"peony":                 "like lilac",
	"pink_petals": `
	m $_1 flowerbed_1 flowerbed=block/$ stem=block/$_stem
	m $_2 flowerbed_2 flowerbed=block/$ stem=block/$_stem
	m $_3 flowerbed_3 flowerbed=block/$ stem=block/$_stem
	m $_4 flowerbed_4 flowerbed=block/$ stem=block/$_stem
	p {"facing":"north"} -> $_1
	p {"facing":"east"} -> $_1 y90
	p {"facing":"south"} -> $_1 y180
	p {"facing":"west"} -> $_1 y270
	p {"facing":"north","flower_amount":"2|3|4"} -> $_2
	p {"facing":"east","flower_amount":"2|3|4"} -> $_2 y90
	p {"facing":"south","flower_amount":"2|3|4"} -> $_2 y180
	p {"facing":"west","flower_amount":"2|3|4"} -> $_2 y270
	p {"facing":"north","flower_amount":"3|4"} -> $_3
	p {"facing":"east","flower_amount":"3|4"} -> $_3 y90
	p {"facing":"south","flower_amount":"3|4"} -> $_3 y180
	p {"facing":"west","flower_amount":"3|4"} -> $_3 y270
	p {"facing":"north","flower_amount":"4"} -> $_4
	p {"facing":"east","flower_amount":"4"} -> $_4 y90
	p {"facing":"south","flower_amount":"4"} -> $_4 y180
	p {"facing":"west","flower_amount":"4"} -> $_4 y270`,
	"piston": `
	m $ template_$ bottom=block/$_bottom platform=block/$_top side=block/$_side
	m $_base $_extended bottom=block/$_bottom inside=block/$_inner side=block/$_side
	v extended=false,facing=down -> $ x90
	v extended=false,facing=east -> $ y90
	v extended=false,facing=north -> $
	v extended=false,facing=south -> $ y180
	v extended=false,facing=up -> $ x270
	v extended=false,facing=west -> $ y270
	v extended=true,facing=down -> $_base x90
	v extended=true,facing=east -> $_base y90
	v extended=true,facing=north -> $_base
	v extended=true,facing=south -> $_base y180
	v extended=true,facing=up -> $_base x270
	v extended=true,facing=west -> $_base y270`,
	"piston_head": `
	m $ template_$ platform=block/piston_top side=block/piston_side unsticky=block/piston_top
	m $_short template_$_short platform=block/piston_top side=block/piston_side unsticky=block/piston_top
	m $_short_sticky template_$_short platform=block/piston_top_sticky side=block/piston_side unsticky=block/piston_top
	m $_sticky template_$ platform=block/piston_top_sticky side=block/piston_side unsticky=block/piston_top
	v facing=down,short=false,type=normal -> $ x90
	v facing=down,short=false,type=sticky -> $_sticky x90
	v facing=down,short=true,type=normal -> $_short x90
	v facing=down,short=true,type=sticky -> $_short_sticky x90
	v facing=east,short=false,type=normal -> $ y90
	v facing=east,short=false,type=sticky -> $_sticky y90
	v facing=east,short=true,type=normal -> $_short y90
	v facing=east,short=true,type=sticky -> $_short_sticky y90
	v facing=north,short=false,type=normal -> $
	v facing=north,short=false,type=sticky -> $_sticky
	v facing=north,short=true,type=normal -> $_short
	v facing=north,short=true,type=sticky -> $_short_sticky
	v facing=south,short=false,type=normal -> $ y180
	v facing=south,short=false,type=sticky -> $_sticky y180
	v facing=south,short=true,type=normal -> $_short y180
	v facing=south,short=true,type=sticky -> $_short_sticky y180
	v facing=up,short=false,type=normal -> $ x270
	v facing=up,short=false,type=sticky -> $_sticky x270
	v facing=up,short=true,type=normal -> $_short x270
	v facing=up,short=true,type=sticky -> $_short_sticky x270
	v facing=west,short=false,type=normal -> $ y270
	v facing=west,short=false,type=sticky -> $_sticky y270
	v facing=west,short=true,type=normal -> $_short y270
	v facing=west,short=true,type=sticky -> $_short_sticky y270`,
	"pitcher_crop": `
	m $_top_stage_0 - ao=false particle=block/$_top
	m $_top_stage_1 - ao=false particle=block/$_top
	m $_top_stage_2 - ao=false particle=block/$_top
	v age=0,half=lower -> $_bottom_stage_0
	v age=0,half=upper -> $_top_stage_0
	v age=1,half=lower -> $_bottom_stage_1
	v age=1,half=upper -> $_top_stage_1
	v age=2,half=lower -> $_bottom_stage_2
	v age=2,half=upper -> $_top_stage_2
	v age=3,half=lower -> $_bottom_stage_3
	v age=3,half=upper -> $_top_stage_3
	v age=4,half=lower -> $_bottom_stage_4
	v age=4,half=upper -> $_top_stage_4`,
	"pitcher_plant": `
	v half=lower -> $_bottom
	v half=upper -> $_top`,
	"podzol": "like mycelium",
	"pointed_dripstone": `
	m $_down_base $ cross=block/$_down_base
	m $_down_frustum $ cross=block/$_down_frustum
	m $_down_middle $ cross=block/$_down_middle
	m $_down_tip $ cross=block/$_down_tip
	m $_down_tip_merge $ cross=block/$_down_tip_merge
	m $_up_base $ cross=block/$_up_base
	m $_up_frustum $ cross=block/$_up_frustum
	m $_up_middle $ cross=block/$_up_middle
	m $_up_tip $ cross=block/$_up_tip
	m $_up_tip_merge $ cross=block/$_up_tip_merge
	v thickness=base,vertical_direction=down -> $_down_base
	v thickness=base,vertical_direction=up -> $_up_base
	v thickness=frustum,vertical_direction=down -> $_down_frustum
	v thickness=frustum,vertical_direction=up -> $_up_frustum
	v thickness=middle,vertical_direction=down -> $_down_middle
	v thickness=middle,vertical_direction=up -> $_up_middle
	v thickness=tip,vertical_direction=down -> $_down_tip
	v thickness=tip,vertical_direction=up -> $_up_tip
	v thickness=tip_merge,vertical_direction=down -> $_down_tip_merge
	v thickness=tip_merge,vertical_direction=up -> $_up_tip_merge`,
	"polished_basalt": "like basalt",
	"potatoes":        "like carrots",
	"powder_snow_cauldron": `
	m $_full template_cauldron_full bottom=block/cauldron_bottom content=block/powder_snow inside=block/cauldron_inner particle=block/cauldron_side side=block/cauldron_side top=block/cauldron_top
	m $_level1 template_cauldron_level1 bottom=block/cauldron_bottom content=block/powder_snow inside=block/cauldron_inner particle=block/cauldron_side side=block/cauldron_side top=block/cauldron_top
	m $_level2 template_cauldron_level2 bottom=block/cauldron_bottom content=block/powder_snow inside=block/cauldron_inner particle=block/cauldron_side side=block/cauldron_side top=block/cauldron_top
	v level=1 -> $_level1
	v level=2 -> $_level2
	v level=3 -> $_full`,
	"powered_rail":  "like activator_rail",
	"pumpkin_stem":  "like melon_stem",
	"purpur_pillar": "like hay_block",
	"quartz_pillar": "like hay_block",
	"rail": `
	m $ $_flat $=block/$
	m $_corner $_curved $=block/$_corner
	m $_raised_ne template_$_raised_ne $=block/$
	m $_raised_sw template_$_raised_sw $=block/$
	v shape=ascending_east -> $_raised_ne y90
	v shape=ascending_north -> $_raised_ne
	v shape=ascending_south -> $_raised_sw
	v shape=ascending_west -> $_raised_sw y90
	v shape=east_west -> $ y90
	v shape=north_east -> $_corner y270
	v shape=north_south -> $
	v shape=north_west -> $_corner y180
	v shape=south_east -> $_corner
	v shape=south_west -> $_corner y90`,
	"red_mushroom_block": "like mushroom_stem",
	"red_sand":           "like dirt",
	"redstone_lamp": `
	m $ cube_all all=block/$
	m $_on cube_all all=block/$_on
	v lit=false -> $
	v lit=true -> $_on`,
	"redstone_torch": `
	m $ template_$ torch=block/$
	m $_off template_torch_unlit torch=block/$_off
	v lit=false -> $_off
	v lit=true -> $`,
	"redstone_wall_torch": `
	m $ template_redstone_torch_wall torch=block/redstone_torch
	m $_off template_torch_wall_unlit torch=block/redstone_torch_off
	v facing=east,lit=false -> $_off
	v facing=east,lit=true -> $
	v facing=north,lit=false -> $_off y270
	v facing=north,lit=true -> $ y270
	v facing=south,lit=false -> $_off y90
	v facing=south,lit=true -> $ y90
	v facing=west,lit=false -> $_off y180
	v facing=west,lit=true -> $ y180`,
	"redstone_wire": `
	m redstone_dust_side0 redstone_dust_side line=block/redstone_dust_line0
	m redstone_dust_side1 redstone_dust_side line=block/redstone_dust_line1
	m redstone_dust_side_alt0 redstone_dust_side_alt line=block/redstone_dust_line0
	m redstone_dust_side_alt1 redstone_dust_side_alt line=block/redstone_dust_line1
	p {"OR":[{"east":"none","north":"none","south":"none","west":"none"},{"east":"side|up","north":"side|up"},{"east":"side|up","south":"side|up"},{"south":"side|up","west":"side|up"},{"north":"side|up","west":"side|up"}]} -> redstone_dust_dot
	p {"north":"side|up"} -> redstone_dust_side0
	p {"south":"side|up"} -> redstone_dust_side_alt0
	p {"east":"side|up"} -> redstone_dust_side_alt1 y270
	p {"west":"side|up"} -> redstone_dust_side1 y270
	p {"north":"up"} -> redstone_dust_up
	p {"east":"up"} -> redstone_dust_up y90
	p {"south":"up"} -> redstone_dust_up y180
	p {"west":"up"} -> redstone_dust_up y270`,
	"repeater": `
	v delay=1,facing=east,locked=false,powered=false -> $_1tick y270
	v delay=1,facing=east,locked=false,powered=true -> $_1tick_on y270
	v delay=1,facing=east,locked=true,powered=false -> $_1tick_locked y270
	v delay=1,facing=east,locked=true,powered=true -> $_1tick_on_locked y270
	v delay=1,facing=north,locked=false,powered=false -> $_1tick y180
	v delay=1,facing=north,locked=false,powered=true -> $_1tick_on y180
	v delay=1,facing=north,locked=true,powered=false -> $_1tick_locked y180
	v delay=1,facing=north,locked=true,powered=true -> $_1tick_on_locked y180
	v delay=1,facing=south,locked=false,powered=false -> $_1tick
	v delay=1,facing=south,locked=false,powered=true -> $_1tick_on
	v delay=1,facing=south,locked=true,powered=false -> $_1tick_locked
	v delay=1,facing=south,locked=true,powered=true -> $_1tick_on_locked
	v delay=1,facing=west,locked=false,powered=false -> $_1tick y90
	v delay=1,facing=west,locked=false,powered=true -> $_1tick_on y90
	v delay=1,facing=west,locked=true,powered=false -> $_1tick_locked y90
	v delay=1,facing=west,locked=true,powered=true -> $_1tick_on_locked y90
	v delay=2,facing=east,locked=false,powered=false -> $_2tick y270
	v delay=2,facing=east,locked=false,powered=true -> $_2tick_on y270
	v delay=2,facing=east,locked=true,powered=false -> $_2tick_locked y270
	v delay=2,facing=east,locked=true,powered=true -> $_2tick_on_locked y270
	v delay=2,facing=north,locked=false,powered=false -> $_2tick y180
	v delay=2,facing=north,locked=false,powered=true -> $_2tick_on y180
	v delay=2,facing=north,locked=true,powered=false -> $_2tick_locked y180
	v delay=2,facing=north,locked=true,powered=true -> $_2tick_on_locked y180
	v delay=2,facing=south,locked=false,powered=false -> $_2tick
	v delay=2,facing=south,locked=false,powered=true -> $_2tick_on
	v delay=2,facing=south,locked=true,powered=false -> $_2tick_locked
	v delay=2,facing=south,locked=true,powered=true -> $_2tick_on_locked
	v delay=2,facing=west,locked=false,powered=false -> $_2tick y90
	v delay=2,facing=west,locked=false,powered=true -> $_2tick_on y90
	v delay=2,facing=west,locked=true,powered=false -> $_2tick_locked y90
	v delay=2,facing=west,locked=true,powered=true -> $_2tick_on_locked y90
	v delay=3,facing=east,locked=false,powered=false -> $_3tick y270
	v delay=3,facing=east,locked=false,powered=true -> $_3tick_on y270
	v delay=3,facing=east,locked=true,powered=false -> $_3tick_locked y270
	v delay=3,facing=east,locked=true,powered=true -> $_3tick_on_locked y270
	v delay=3,facing=north,locked=false,powered=false -> $_3tick y180
	v delay=3,facing=north,locked=false,powered=true -> $_3tick_on y180
	v delay=3,facing=north,locked=true,powered=false -> $_3tick_locked y180
	v delay=3,facing=north,locked=true,powered=true -> $_3tick_on_locked y180
	v delay=3,facing=south,locked=false,powered=false -> $_3tick
	v delay=3,facing=south,locked=false,powered=true -> $_3tick_on
	v delay=3,facing=south,locked=true,powered=false -> $_3tick_locked
	v delay=3,facing=south,locked=true,powered=true -> $_3tick_on_locked
	v delay=3,facing=west,locked=false,powered=false -> $_3tick y90
	v delay=3,facing=west,locked=false,powered=true -> $_3tick_on y90
	v delay=3,facing=west,locked=true,powered=false -> $_3tick_locked y90
	v delay=3,facing=west,locked=true,powered=true -> $_3tick_on_locked y90
	v delay=4,facing=east,locked=false,powered=false -> $_4tick y270
	v delay=4,facing=east,locked=false,powered=true -> $_4tick_on y270
	v delay=4,facing=east,locked=true,powered=false -> $_4tick_locked y270
	v delay=4,facing=east,locked=true,powered=true -> $_4tick_on_locked y270
	v delay=4,facing=north,locked=false,powered=false -> $_4tick y180
	v delay=4,facing=north,locked=false,powered=true -> $_4tick_on y180
	v delay=4,facing=north,locked=true,powered=false -> $_4tick_locked y180
	v delay=4,facing=north,locked=true,powered=true -> $_4tick_on_locked y180
	v delay=4,facing=south,locked=false,powered=false -> $_4tick
	v delay=4,facing=south,locked=false,powered=true -> $_4tick_on
	v delay=4,facing=south,locked=true,powered=false -> $_4tick_locked
	v delay=4,facing=south,locked=true,powered=true -> $_4tick_on_locked
	v delay=4,facing=west,locked=false,powered=false -> $_4tick y90
	v delay=4,facing=west,locked=false,powered=true -> $_4tick_on y90
	v delay=4,facing=west,locked=true,powered=false -> $_4tick_locked y90
	v delay=4,facing=west,locked=true,powered=true -> $_4tick_on_locked y90`,
	"repeating_command_block": "like chain_command_block",
	"resin_clump":             "like glow_lichen",
	"respawn_anchor": `
	m $_0 cube_bottom_top bottom=block/$_bottom side=block/$_side0 top=block/$_top_off
	m $_1 cube_bottom_top bottom=block/$_bottom side=block/$_side1 top=block/$_top
	m $_2 cube_bottom_top bottom=block/$_bottom side=block/$_side2 top=block/$_top
	m $_3 cube_bottom_top bottom=block/$_bottom side=block/$_side3 top=block/$_top
	m $_4 cube_bottom_top bottom=block/$_bottom side=block/$_side4 top=block/$_top
	v charges=0 -> $_0
	v charges=1 -> $_1
	v charges=2 -> $_2
	v charges=3 -> $_3
	v charges=4 -> $_4`,
	"rooted_dirt": "like dirt",
	"rose_bush":   "like lilac",
	"sand":        "like dirt",
	"scaffolding": `
	v bottom=false -> $_stable
	v bottom=true -> $_unstable`,
	"sculk": "like bedrock",
	"sculk_catalyst": `
	m $ cube_bottom_top bottom=block/$_bottom side=block/$_side top=block/$_top
	m $_bloom cube_bottom_top bottom=block/$_bottom side=block/$_side_bloom top=block/$_top_bloom
	v bloom=false -> $
	v bloom=true -> $_bloom`,
	"sculk_sensor": `
	m $_active $ tendrils=block/$_tendril_active
	m $_inactive $ tendrils=block/$_tendril_inactive
	v sculk_sensor_phase=active -> $_active
	v sculk_sensor_phase=cooldown -> $_active
	v sculk_sensor_phase=inactive -> $_inactive`,
	"sculk_shrieker": `
	m $ template_$ bottom=block/$_bottom inner_top=block/$_inner_top particle=block/$_bottom side=block/$_side top=block/$_top
	m $_can_summon template_$ bottom=block/$_bottom inner_top=block/$_can_summon_inner_top particle=block/$_bottom side=block/$_side top=block/$_top
	v can_summon=false -> $
	v can_summon=true -> $_can_summon`,
	"sculk_vein": "like glow_lichen",
	"sea_pickle": `
	v pickles=1,waterlogged=false -> dead_$ | dead_$ y90 | dead_$ y180 | dead_$ y270
	v pickles=1,waterlogged=true -> $ | $ y90 | $ y180 | $ y270
	v pickles=2,waterlogged=false -> two_dead_$s | two_dead_$s y90 | two_dead_$s y180 | two_dead_$s y270
	v pickles=2,waterlogged=true -> two_$s | two_$s y90 | two_$s y180 | two_$s y270
	v pickles=3,waterlogged=false -> three_dead_$s | three_dead_$s y90 | three_dead_$s y180 | three_dead_$s y270
	v pickles=3,waterlogged=true -> three_$s | three_$s y90 | three_$s y180 | three_$s y270
	v pickles=4,waterlogged=false -> four_dead_$s | four_dead_$s y90 | four_dead_$s y180 | four_dead_$s y270
	v pickles=4,waterlogged=true -> four_$s | four_$s y90 | four_$s y180 | four_$s y270`,
	"small_amethyst_bud": "like amethyst_cluster",
	"small_dripleaf": `
	v facing=east,half=lower -> $_bottom y90
	v facing=east,half=upper -> $_top y90
	v facing=north,half=lower -> $_bottom
	v facing=north,half=upper -> $_top
	v facing=south,half=lower -> $_bottom y180
	v facing=south,half=upper -> $_top y180
	v facing=west,half=lower -> $_bottom y270
	v facing=west,half=upper -> $_top y270`,
	"smoker": `
	m $ orientable_with_bottom bottom=block/$_bottom front=block/$_front side=block/$_side top=block/$_top
	m $_on orientable_with_bottom bottom=block/$_bottom front=block/$_front_on side=block/$_side top=block/$_top
	v facing=east,lit=false -> $ y90
	v facing=east,lit=true -> $_on y90
	v facing=north,lit=false -> $
	v facing=north,lit=true -> $_on
	v facing=south,lit=false -> $ y180
	v facing=south,lit=true -> $_on y180
	v facing=west,lit=false -> $ y270
	v facing=west,lit=true -> $_on y270`,
	"smooth_stone_slab": `
	m $ slab bottom=block/smooth_stone side=block/$_side top=block/smooth_stone
	m $_double cube_column end=block/smooth_stone side=block/$_side
	m $_top slab_top bottom=block/smooth_stone side=block/$_side top=block/smooth_stone
	v type=bottom -> $
	v type=double -> $_double
	v type=top -> $_top`,
	"sniffer_egg": `
	m $_not_cracked $ bottom=block/$_not_cracked_bottom east=block/$_not_cracked_east north=block/$_not_cracked_north south=block/$_not_cracked_south top=block/$_not_cracked_top west=block/$_not_cracked_west
	m $_slightly_cracked $ bottom=block/$_slightly_cracked_bottom east=block/$_slightly_cracked_east north=block/$_slightly_cracked_north south=block/$_slightly_cracked_south top=block/$_slightly_cracked_top west=block/$_slightly_cracked_west
	m $_very_cracked $ bottom=block/$_very_cracked_bottom east=block/$_very_cracked_east north=block/$_very_cracked_north south=block/$_very_cracked_south top=block/$_very_cracked_top west=block/$_very_cracked_west
	v hatch=0 -> $_not_cracked
	v hatch=1 -> $_slightly_cracked
	v hatch=2 -> $_very_cracked`,
	"snow": `
	m $_block cube_all all=block/$
	v layers=1 -> $_height2
	v layers=2 -> $_height4
	v layers=3 -> $_height6
	v layers=4 -> $_height8
	v layers=5 -> $_height10
	v layers=6 -> $_height12
	v layers=7 -> $_height14
	v layers=8 -> $_block`,
	"soul_campfire": `
	m $ template_campfire fire=block/$_fire lit_log=block/$_log_lit
	v facing=east,lit=false -> campfire_off y270
	v facing=east,lit=true -> $ y270
	v facing=north,lit=false -> campfire_off y180
	v facing=north,lit=true -> $ y180
	v facing=south,lit=false -> campfire_off
	v facing=south,lit=true -> $
	v facing=west,lit=false -> campfire_off y90
	v facing=west,lit=true -> $ y90`,
	"soul_fire": `
	m $_floor0 template_fire_floor fire=block/$_0
	m $_floor1 template_fire_floor fire=block/$_1
	m $_side0 template_fire_side fire=block/$_0
	m $_side1 template_fire_side fire=block/$_1
	m $_side_alt0 template_fire_side_alt fire=block/$_0
	m $_side_alt1 template_fire_side_alt fire=block/$_1
	p - -> $_floor0 | $_floor1
	p - -> $_side0 | $_side1 | $_side_alt0 | $_side_alt1
	p - -> $_side0 y90 | $_side1 y90 | $_side_alt0 y90 | $_side_alt1 y90
	p - -> $_side0 y180 | $_side1 y180 | $_side_alt0 y180 | $_side_alt1 y180
	p - -> $_side0 y270 | $_side1 y270 | $_side_alt0 y270 | $_side_alt1 y270`,
	"sticky_piston": `
	m piston_base piston_extended bottom=block/piston_bottom inside=block/piston_inner side=block/piston_side
	m $ template_piston bottom=block/piston_bottom platform=block/piston_top_sticky side=block/piston_side
	v extended=false,facing=down -> $ x90
	v extended=false,facing=east -> $ y90
	v extended=false,facing=north -> $
	v extended=false,facing=south -> $ y180
	v extended=false,facing=up -> $ x270
	v extended=false,facing=west -> $ y270
	v extended=true,facing=down -> piston_base x90
	v extended=true,facing=east -> piston_base y90
	v extended=true,facing=north -> piston_base
	v extended=true,facing=south -> piston_base y180
	v extended=true,facing=up -> piston_base x270
	v extended=true,facing=west -> piston_base y270`,
	"stone":                 "like bedrock",
	"stonecutter":           "like big_dripleaf_stem",
	"stripped_bamboo_block": "like bamboo_block",
	"stripped_cherry_log":   "like bamboo_block",
	"stripped_crimson_stem": "like crimson_stem",
	"stripped_warped_stem":  "like crimson_stem",
	"structure_block": `
	m $_corner cube_all all=block/$_corner
	m $_data cube_all all=block/$_data
	m $_load cube_all all=block/$_load
	m $_save cube_all all=block/$_save
	v mode=corner -> $_corner
	v mode=data -> $_data
	v mode=load -> $_load
	v mode=save -> $_save`,
	"sulfur_spike": `
	m $_down_base pointed_dripstone cross=block/$_down_base
	m $_down_frustum pointed_dripstone cross=block/$_down_frustum
	m $_down_middle pointed_dripstone cross=block/$_down_middle
	m $_down_tip pointed_dripstone cross=block/$_down_tip
	m $_down_tip_merge pointed_dripstone cross=block/$_down_tip_merge
	m $_up_base pointed_dripstone cross=block/$_up_base
	m $_up_frustum pointed_dripstone cross=block/$_up_frustum
	m $_up_middle pointed_dripstone cross=block/$_up_middle
	m $_up_tip pointed_dripstone cross=block/$_up_tip
	m $_up_tip_merge pointed_dripstone cross=block/$_up_tip_merge
	v thickness=base,vertical_direction=down -> $_down_base
	v thickness=base,vertical_direction=up -> $_up_base
	v thickness=frustum,vertical_direction=down -> $_down_frustum
	v thickness=frustum,vertical_direction=up -> $_up_frustum
	v thickness=middle,vertical_direction=down -> $_down_middle
	v thickness=middle,vertical_direction=up -> $_up_middle
	v thickness=tip,vertical_direction=down -> $_down_tip
	v thickness=tip,vertical_direction=up -> $_up_tip
	v thickness=tip_merge,vertical_direction=down -> $_down_tip_merge
	v thickness=tip_merge,vertical_direction=up -> $_up_tip_merge`,
	"sunflower": `
	m $_bottom cross cross=block/$_bottom
	v half=lower -> $_bottom
	v half=upper -> $_top`,
	"suspicious_gravel": `
	m $_0 cube_all all=block/$_0
	m $_1 cube_all all=block/$_1
	m $_2 cube_all all=block/$_2
	m $_3 cube_all all=block/$_3
	v dusted=0 -> $_0
	v dusted=1 -> $_1
	v dusted=2 -> $_2
	v dusted=3 -> $_3`,
	"suspicious_sand": "like suspicious_gravel",
	"sweet_berry_bush": `
	m $_stage0 cross cross=block/$_stage0
	m $_stage1 cross cross=block/$_stage1
	m $_stage2 cross cross=block/$_stage2
	m $_stage3 cross cross=block/$_stage3
	v age=0 -> $_stage0
	v age=1 -> $_stage1
	v age=2 -> $_stage2
	v age=3 -> $_stage3`,
	"tall_grass": "like large_fern",
	"tall_seagrass": `
	m $_bottom template_seagrass texture=block/$_bottom
	m $_top template_seagrass texture=block/$_top
	v half=lower -> $_bottom
	v half=upper -> $_top`,
	"test_block": `
	m $_accept cube_all all=block/$_accept
	m $_fail cube_all all=block/$_fail
	m $_log cube_all all=block/$_log
	m $_start cube_all all=block/$_start
	v mode=accept -> $_accept
	v mode=fail -> $_fail
	v mode=log -> $_log
	v mode=start -> $_start`,
	"torchflower_crop": `
	m $_stage0 cross cross=block/$_stage0
	m $_stage1 cross cross=block/$_stage1
	v age=0 -> $_stage0
	v age=1 -> $_stage1`,
	"trial_spawner": `
	m $ cube_bottom_top_inner_faces bottom=block/$_bottom side=block/$_side_inactive top=block/$_top_inactive
	m $_active cube_bottom_top_inner_faces bottom=block/$_bottom side=block/$_side_active top=block/$_top_active
	m $_active_ominous cube_bottom_top_inner_faces bottom=block/$_bottom side=block/$_side_active_ominous top=block/$_top_active_ominous
	m $_ejecting_reward cube_bottom_top_inner_faces bottom=block/$_bottom side=block/$_side_active top=block/$_top_ejecting_reward
	m $_ejecting_reward_ominous cube_bottom_top_inner_faces bottom=block/$_bottom side=block/$_side_active_ominous top=block/$_top_ejecting_reward_ominous
	m $_inactive_ominous cube_bottom_top_inner_faces bottom=block/$_bottom side=block/$_side_inactive_ominous top=block/$_top_inactive_ominous
	v ominous=false,trial_spawner_state=active -> $_active
	v ominous=false,trial_spawner_state=cooldown -> $
	v ominous=false,trial_spawner_state=ejecting_reward -> $_ejecting_reward
	v ominous=false,trial_spawner_state=inactive -> $
	v ominous=false,trial_spawner_state=waiting_for_players -> $_active
	v ominous=false,trial_spawner_state=waiting_for_reward_ejection -> $_active
	v ominous=true,trial_spawner_state=active -> $_active_ominous
	v ominous=true,trial_spawner_state=cooldown -> $_inactive_ominous
	v ominous=true,trial_spawner_state=ejecting_reward -> $_ejecting_reward_ominous
	v ominous=true,trial_spawner_state=inactive -> $_inactive_ominous
	v ominous=true,trial_spawner_state=waiting_for_players -> $_active_ominous
	v ominous=true,trial_spawner_state=waiting_for_reward_ejection -> $_active_ominous`,
	"tripwire": `
	v attached=false,east=false,north=false,south=false,west=false -> $_ns
	v attached=false,east=false,north=false,south=false,west=true -> $_n y270
	v attached=false,east=false,north=false,south=true,west=false -> $_n y180
	v attached=false,east=false,north=false,south=true,west=true -> $_ne y180
	v attached=false,east=false,north=true,south=false,west=false -> $_n
	v attached=false,east=false,north=true,south=false,west=true -> $_ne y270
	v attached=false,east=false,north=true,south=true,west=false -> $_ns
	v attached=false,east=false,north=true,south=true,west=true -> $_nse y180
	v attached=false,east=true,north=false,south=false,west=false -> $_n y90
	v attached=false,east=true,north=false,south=false,west=true -> $_ns y90
	v attached=false,east=true,north=false,south=true,west=false -> $_ne y90
	v attached=false,east=true,north=false,south=true,west=true -> $_nse y90
	v attached=false,east=true,north=true,south=false,west=false -> $_ne
	v attached=false,east=true,north=true,south=false,west=true -> $_nse y270
	v attached=false,east=true,north=true,south=true,west=false -> $_nse
	v attached=false,east=true,north=true,south=true,west=true -> $_nsew
	v attached=true,east=false,north=false,south=false,west=false -> $_attached_ns
	v attached=true,east=false,north=false,south=false,west=true -> $_attached_n y270
	v attached=true,east=false,north=false,south=true,west=false -> $_attached_n y180
	v attached=true,east=false,north=false,south=true,west=true -> $_attached_ne y180
	v attached=true,east=false,north=true,south=false,west=false -> $_attached_n
	v attached=true,east=false,north=true,south=false,west=true -> $_attached_ne y270
	v attached=true,east=false,north=true,south=true,west=false -> $_attached_ns
	v attached=true,east=false,north=true,south=true,west=true -> $_attached_nse y180
	v attached=true,east=true,north=false,south=false,west=false -> $_attached_n y90
	v attached=true,east=true,north=false,south=false,west=true -> $_attached_ns y90
	v attached=true,east=true,north=false,south=true,west=false -> $_attached_ne y90
	v attached=true,east=true,north=false,south=true,west=true -> $_attached_nse y90
	v attached=true,east=true,north=true,south=false,west=false -> $_attached_ne
	v attached=true,east=true,north=true,south=false,west=true -> $_attached_nse y270
	v attached=true,east=true,north=true,south=true,west=false -> $_attached_nse
	v attached=true,east=true,north=true,south=true,west=true -> $_attached_nsew`,
	"tripwire_hook": `
	v attached=false,facing=east,powered=false -> $ y90
	v attached=false,facing=east,powered=true -> $_on y90
	v attached=false,facing=north,powered=false -> $
	v attached=false,facing=north,powered=true -> $_on
	v attached=false,facing=south,powered=false -> $ y180
	v attached=false,facing=south,powered=true -> $_on y180
	v attached=false,facing=west,powered=false -> $ y270
	v attached=false,facing=west,powered=true -> $_on y270
	v attached=true,facing=east,powered=false -> $_attached y90
	v attached=true,facing=east,powered=true -> $_attached_on y90
	v attached=true,facing=north,powered=false -> $_attached
	v attached=true,facing=north,powered=true -> $_attached_on
	v attached=true,facing=south,powered=false -> $_attached y180
	v attached=true,facing=south,powered=true -> $_attached_on y180
	v attached=true,facing=west,powered=false -> $_attached y270
	v attached=true,facing=west,powered=true -> $_attached_on y270`,
	"turtle_egg": `
	m four_slightly_cracked_$s template_four_$s all=block/$_slightly_cracked
	m four_$s template_four_$s all=block/$
	m four_very_cracked_$s template_four_$s all=block/$_very_cracked
	m slightly_cracked_$ template_$ all=block/$_slightly_cracked
	m three_slightly_cracked_$s template_three_$s all=block/$_slightly_cracked
	m three_$s template_three_$s all=block/$
	m three_very_cracked_$s template_three_$s all=block/$_very_cracked
	m $ template_$ all=block/$
	m two_slightly_cracked_$s template_two_$s all=block/$_slightly_cracked
	m two_$s template_two_$s all=block/$
	m two_very_cracked_$s template_two_$s all=block/$_very_cracked
	m very_cracked_$ template_$ all=block/$_very_cracked
	v eggs=1,hatch=0 -> $ | $ y90 | $ y180 | $ y270
	v eggs=1,hatch=1 -> slightly_cracked_$ | slightly_cracked_$ y90 | slightly_cracked_$ y180 | slightly_cracked_$ y270
	v eggs=1,hatch=2 -> very_cracked_$ | very_cracked_$ y90 | very_cracked_$ y180 | very_cracked_$ y270
	v eggs=2,hatch=0 -> two_$s | two_$s y90 | two_$s y180 | two_$s y270
	v eggs=2,hatch=1 -> two_slightly_cracked_$s | two_slightly_cracked_$s y90 | two_slightly_cracked_$s y180 | two_slightly_cracked_$s y270
	v eggs=2,hatch=2 -> two_very_cracked_$s | two_very_cracked_$s y90 | two_very_cracked_$s y180 | two_very_cracked_$s y270
	v eggs=3,hatch=0 -> three_$s | three_$s y90 | three_$s y180 | three_$s y270
	v eggs=3,hatch=1 -> three_slightly_cracked_$s | three_slightly_cracked_$s y90 | three_slightly_cracked_$s y180 | three_slightly_cracked_$s y270
	v eggs=3,hatch=2 -> three_very_cracked_$s | three_very_cracked_$s y90 | three_very_cracked_$s y180 | three_very_cracked_$s y270
	v eggs=4,hatch=0 -> four_$s | four_$s y90 | four_$s y180 | four_$s y270
	v eggs=4,hatch=1 -> four_slightly_cracked_$s | four_slightly_cracked_$s y90 | four_slightly_cracked_$s y180 | four_slightly_cracked_$s y270
	v eggs=4,hatch=2 -> four_very_cracked_$s | four_very_cracked_$s y90 | four_very_cracked_$s y180 | four_very_cracked_$s y270`,
	"vault": `
	m $ template_$ bottom=block/$_bottom front=block/$_front_off side=block/$_side_off top=block/$_top
	m $_active template_$ bottom=block/$_bottom front=block/$_front_on side=block/$_side_on top=block/$_top
	m $_active_ominous template_$ bottom=block/$_bottom_ominous front=block/$_front_on_ominous side=block/$_side_on_ominous top=block/$_top_ominous
	m $_ejecting_reward template_$ bottom=block/$_bottom front=block/$_front_ejecting side=block/$_side_on top=block/$_top_ejecting
	m $_ejecting_reward_ominous template_$ bottom=block/$_bottom_ominous front=block/$_front_ejecting_ominous side=block/$_side_on_ominous top=block/$_top_ejecting_ominous
	m $_ominous template_$ bottom=block/$_bottom_ominous front=block/$_front_off_ominous side=block/$_side_off_ominous top=block/$_top_ominous
	m $_unlocking template_$ bottom=block/$_bottom front=block/$_front_ejecting side=block/$_side_on top=block/$_top
	m $_unlocking_ominous template_$ bottom=block/$_bottom_ominous front=block/$_front_ejecting_ominous side=block/$_side_on_ominous top=block/$_top_ominous
	v facing=east,ominous=false,vault_state=active -> $_active y90
	v facing=east,ominous=false,vault_state=ejecting -> $_ejecting_reward y90
	v facing=east,ominous=false,vault_state=inactive -> $ y90
	v facing=east,ominous=false,vault_state=unlocking -> $_unlocking y90
	v facing=east,ominous=true,vault_state=active -> $_active_ominous y90
	v facing=east,ominous=true,vault_state=ejecting -> $_ejecting_reward_ominous y90
	v facing=east,ominous=true,vault_state=inactive -> $_ominous y90
	v facing=east,ominous=true,vault_state=unlocking -> $_unlocking_ominous y90
	v facing=north,ominous=false,vault_state=active -> $_active
	v facing=north,ominous=false,vault_state=ejecting -> $_ejecting_reward
	v facing=north,ominous=false,vault_state=inactive -> $
	v facing=north,ominous=false,vault_state=unlocking -> $_unlocking
	v facing=north,ominous=true,vault_state=active -> $_active_ominous
	v facing=north,ominous=true,vault_state=ejecting -> $_ejecting_reward_ominous
	v facing=north,ominous=true,vault_state=inactive -> $_ominous
	v facing=north,ominous=true,vault_state=unlocking -> $_unlocking_ominous
	v facing=south,ominous=false,vault_state=active -> $_active y180
	v facing=south,ominous=false,vault_state=ejecting -> $_ejecting_reward y180
	v facing=south,ominous=false,vault_state=inactive -> $ y180
	v facing=south,ominous=false,vault_state=unlocking -> $_unlocking y180
	v facing=south,ominous=true,vault_state=active -> $_active_ominous y180
	v facing=south,ominous=true,vault_state=ejecting -> $_ejecting_reward_ominous y180
	v facing=south,ominous=true,vault_state=inactive -> $_ominous y180
	v facing=south,ominous=true,vault_state=unlocking -> $_unlocking_ominous y180
	v facing=west,ominous=false,vault_state=active -> $_active y270
	v facing=west,ominous=false,vault_state=ejecting -> $_ejecting_reward y270
	v facing=west,ominous=false,vault_state=inactive -> $ y270
	v facing=west,ominous=false,vault_state=unlocking -> $_unlocking y270
	v facing=west,ominous=true,vault_state=active -> $_active_ominous y270
	v facing=west,ominous=true,vault_state=ejecting -> $_ejecting_reward_ominous y270
	v facing=west,ominous=true,vault_state=inactive -> $_ominous y270
	v facing=west,ominous=true,vault_state=unlocking -> $_unlocking_ominous y270`,
	"verdant_froglight": "like hay_block",
	"vine": `
	p {"north":"true"} -> $
	p {"east":"false","north":"false","south":"false","up":"false","west":"false"} -> $
	p {"east":"true"} -> $ y90 L
	p {"east":"false","north":"false","south":"false","up":"false","west":"false"} -> $ y90 L
	p {"south":"true"} -> $ y180 L
	p {"east":"false","north":"false","south":"false","up":"false","west":"false"} -> $ y180 L
	p {"west":"true"} -> $ y270 L
	p {"east":"false","north":"false","south":"false","up":"false","west":"false"} -> $ y270 L
	p {"up":"true"} -> $ x270 L
	p {"east":"false","north":"false","south":"false","up":"false","west":"false"} -> $ x270 L`,
	"void_air":    "like cave_air",
	"warped_stem": "like crimson_stem",
	"water_cauldron": `
	m $_full template_cauldron_full bottom=block/cauldron_bottom content=block/water_still inside=block/cauldron_inner particle=block/cauldron_side side=block/cauldron_side top=block/cauldron_top
	m $_level1 template_cauldron_level1 bottom=block/cauldron_bottom content=block/water_still inside=block/cauldron_inner particle=block/cauldron_side side=block/cauldron_side top=block/cauldron_top
	m $_level2 template_cauldron_level2 bottom=block/cauldron_bottom content=block/water_still inside=block/cauldron_inner particle=block/cauldron_side side=block/cauldron_side top=block/cauldron_top
	v level=1 -> $_level1
	v level=2 -> $_level2
	v level=3 -> $_full`,
	"waxed_copper_bulb": `
	m copper_bulb cube_all all=block/copper_bulb
	m copper_bulb_lit cube_all all=block/copper_bulb_lit
	m copper_bulb_lit_powered cube_all all=block/copper_bulb_lit_powered
	m copper_bulb_powered cube_all all=block/copper_bulb_powered
	v lit=false,powered=false -> copper_bulb
	v lit=false,powered=true -> copper_bulb_powered
	v lit=true,powered=false -> copper_bulb_lit
	v lit=true,powered=true -> copper_bulb_lit_powered`,
	"waxed_copper_chain": `
	m copper_chain template_chain texture=block/copper_chain
	v axis=x -> copper_chain x90 y90
	v axis=y -> copper_chain
	v axis=z -> copper_chain x90`,
	"waxed_exposed_copper_bulb": `
	m exposed_copper_bulb cube_all all=block/exposed_copper_bulb
	m exposed_copper_bulb_lit cube_all all=block/exposed_copper_bulb_lit
	m exposed_copper_bulb_lit_powered cube_all all=block/exposed_copper_bulb_lit_powered
	m exposed_copper_bulb_powered cube_all all=block/exposed_copper_bulb_powered
	v lit=false,powered=false -> exposed_copper_bulb
	v lit=false,powered=true -> exposed_copper_bulb_powered
	v lit=true,powered=false -> exposed_copper_bulb_lit
	v lit=true,powered=true -> exposed_copper_bulb_lit_powered`,
	"waxed_exposed_copper_chain": `
	m exposed_copper_chain template_chain texture=block/exposed_copper_chain
	v axis=x -> exposed_copper_chain x90 y90
	v axis=y -> exposed_copper_chain
	v axis=z -> exposed_copper_chain x90`,
	"waxed_exposed_lightning_rod": `
	m exposed_lightning_rod template_lightning_rod texture=block/exposed_lightning_rod
	m lightning_rod_on template_lightning_rod texture=block/lightning_rod_on
	v facing=down,powered=false -> exposed_lightning_rod x180
	v facing=down,powered=true -> lightning_rod_on x180
	v facing=east,powered=false -> exposed_lightning_rod x90 y90
	v facing=east,powered=true -> lightning_rod_on x90 y90
	v facing=north,powered=false -> exposed_lightning_rod x90
	v facing=north,powered=true -> lightning_rod_on x90
	v facing=south,powered=false -> exposed_lightning_rod x90 y180
	v facing=south,powered=true -> lightning_rod_on x90 y180
	v facing=up,powered=false -> exposed_lightning_rod
	v facing=up,powered=true -> lightning_rod_on
	v facing=west,powered=false -> exposed_lightning_rod x90 y270
	v facing=west,powered=true -> lightning_rod_on x90 y270`,
	"waxed_lightning_rod": `
	m lightning_rod template_lightning_rod texture=block/lightning_rod
	m lightning_rod_on template_lightning_rod texture=block/lightning_rod_on
	v facing=down,powered=false -> lightning_rod x180
	v facing=down,powered=true -> lightning_rod_on x180
	v facing=east,powered=false -> lightning_rod x90 y90
	v facing=east,powered=true -> lightning_rod_on x90 y90
	v facing=north,powered=false -> lightning_rod x90
	v facing=north,powered=true -> lightning_rod_on x90
	v facing=south,powered=false -> lightning_rod x90 y180
	v facing=south,powered=true -> lightning_rod_on x90 y180
	v facing=up,powered=false -> lightning_rod
	v facing=up,powered=true -> lightning_rod_on
	v facing=west,powered=false -> lightning_rod x90 y270
	v facing=west,powered=true -> lightning_rod_on x90 y270`,
	"waxed_oxidized_copper_bulb": `
	m oxidized_copper_bulb cube_all all=block/oxidized_copper_bulb
	m oxidized_copper_bulb_lit cube_all all=block/oxidized_copper_bulb_lit
	m oxidized_copper_bulb_lit_powered cube_all all=block/oxidized_copper_bulb_lit_powered
	m oxidized_copper_bulb_powered cube_all all=block/oxidized_copper_bulb_powered
	v lit=false,powered=false -> oxidized_copper_bulb
	v lit=false,powered=true -> oxidized_copper_bulb_powered
	v lit=true,powered=false -> oxidized_copper_bulb_lit
	v lit=true,powered=true -> oxidized_copper_bulb_lit_powered`,
	"waxed_oxidized_copper_chain": `
	m oxidized_copper_chain template_chain texture=block/oxidized_copper_chain
	v axis=x -> oxidized_copper_chain x90 y90
	v axis=y -> oxidized_copper_chain
	v axis=z -> oxidized_copper_chain x90`,
	"waxed_oxidized_lightning_rod": `
	m lightning_rod_on template_lightning_rod texture=block/lightning_rod_on
	m oxidized_lightning_rod template_lightning_rod texture=block/oxidized_lightning_rod
	v facing=down,powered=false -> oxidized_lightning_rod x180
	v facing=down,powered=true -> lightning_rod_on x180
	v facing=east,powered=false -> oxidized_lightning_rod x90 y90
	v facing=east,powered=true -> lightning_rod_on x90 y90
	v facing=north,powered=false -> oxidized_lightning_rod x90
	v facing=north,powered=true -> lightning_rod_on x90
	v facing=south,powered=false -> oxidized_lightning_rod x90 y180
	v facing=south,powered=true -> lightning_rod_on x90 y180
	v facing=up,powered=false -> oxidized_lightning_rod
	v facing=up,powered=true -> lightning_rod_on
	v facing=west,powered=false -> oxidized_lightning_rod x90 y270
	v facing=west,powered=true -> lightning_rod_on x90 y270`,
	"waxed_weathered_copper_bulb": `
	m weathered_copper_bulb cube_all all=block/weathered_copper_bulb
	m weathered_copper_bulb_lit cube_all all=block/weathered_copper_bulb_lit
	m weathered_copper_bulb_lit_powered cube_all all=block/weathered_copper_bulb_lit_powered
	m weathered_copper_bulb_powered cube_all all=block/weathered_copper_bulb_powered
	v lit=false,powered=false -> weathered_copper_bulb
	v lit=false,powered=true -> weathered_copper_bulb_powered
	v lit=true,powered=false -> weathered_copper_bulb_lit
	v lit=true,powered=true -> weathered_copper_bulb_lit_powered`,
	"waxed_weathered_copper_chain": `
	m weathered_copper_chain template_chain texture=block/weathered_copper_chain
	v axis=x -> weathered_copper_chain x90 y90
	v axis=y -> weathered_copper_chain
	v axis=z -> weathered_copper_chain x90`,
	"waxed_weathered_lightning_rod": `
	m lightning_rod_on template_lightning_rod texture=block/lightning_rod_on
	m weathered_lightning_rod template_lightning_rod texture=block/weathered_lightning_rod
	v facing=down,powered=false -> weathered_lightning_rod x180
	v facing=down,powered=true -> lightning_rod_on x180
	v facing=east,powered=false -> weathered_lightning_rod x90 y90
	v facing=east,powered=true -> lightning_rod_on x90 y90
	v facing=north,powered=false -> weathered_lightning_rod x90
	v facing=north,powered=true -> lightning_rod_on x90
	v facing=south,powered=false -> weathered_lightning_rod x90 y180
	v facing=south,powered=true -> lightning_rod_on x90 y180
	v facing=up,powered=false -> weathered_lightning_rod
	v facing=up,powered=true -> lightning_rod_on
	v facing=west,powered=false -> weathered_lightning_rod x90 y270
	v facing=west,powered=true -> lightning_rod_on x90 y270`,
	"weathered_copper_bulb":   "like copper_bulb",
	"weathered_copper_chain":  "like copper_chain",
	"weathered_lightning_rod": "like oxidized_lightning_rod",
	"wheat": `
	m $_stage0 crop crop=block/$_stage0
	m $_stage1 crop crop=block/$_stage1
	m $_stage2 crop crop=block/$_stage2
	m $_stage3 crop crop=block/$_stage3
	m $_stage4 crop crop=block/$_stage4
	m $_stage5 crop crop=block/$_stage5
	m $_stage6 crop crop=block/$_stage6
	m $_stage7 crop crop=block/$_stage7
	v age=0 -> $_stage0
	v age=1 -> $_stage1
	v age=2 -> $_stage2
	v age=3 -> $_stage3
	v age=4 -> $_stage4
	v age=5 -> $_stage5
	v age=6 -> $_stage6
	v age=7 -> $_stage7`,
	"wildflowers": "like pink_petals",
}
