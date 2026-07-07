# Facility “Other” bucket — recipe_outputs coverage gap

_Data audit generated from `catalog_facilities.json` + `crafting.db`. Not a KB page._

The facilities build-cost **Other** group is **186 production facilities** across **77 recipes** whose `recipe_id` has **no row in `recipe_outputs`**, so the KB can't tell what they produce and falls them back to “Other”. Populate `recipe_outputs` for each recipe below (or remove the facility if it has no craftable output) and they will regroup by produced-item category on regen.

Faction/pirate facilities are already distinguishable via the catalog's `empire` / `pirate_base_only` fields — e.g. **The Wire Room** (`empire: pirates`, `unique: true`). Only that one facility here is faction-restricted; the rest are ordinary crafters missing an output link.

| recipe_id | # facilities | levels | facility ids | faction |
|---|--:|---|---|---|
| `adamant_tooth_forging` | 1 | L2 | adamant_tooth_furnace |  |
| `anchor_plate_forging` | 1 | L1 | barnacle_plate_forge |  |
| `assemble_gold_processing_core` | 4 | L1/L2/L3/L4 | gold_core_assembly, gold_interconnect_plant, auric_compute_foundry, gilded_cognition_works |  |
| `assemble_outpost_kit` | 1 | L2 | outpost_frame_assembler |  |
| `assemble_station_core` | 1 | L4 | station_core_foundry |  |
| `baleen_capital_plate` | 1 | L2 | baleen_plate_works |  |
| `biogas_fuel_synthesis` | 2 | L1/L2 | biogas_cracker, biogas_distillation_plant |  |
| `bond_iridium_silicate_composite` | 4 | L1/L2/L3/L4 | iridium_sinter_kiln, iridium_composite_furnace, iridium_silicate_reactor, iridium_sinter_array |  |
| `build_carbon_scrubber` | 4 | L1/L2/L3/L4 | reactive_bed_bench, intake_filter_module_plant, carbon_separation_fabrication_complex, automated_scrubber_works |  |
| `build_compact_aluminum_smelter` | 4 | L1/L2/L3/L4 | assembly_bay_for_compact_aluminum_smelter, fabrication_plant_for_compact_aluminum_smelter, integration_works_for_compact_aluminum_smelter, outfitting_manufactory_for_compact_aluminum_smelter |  |
| `build_compact_argon_purifier` | 4 | L1/L2/L3/L4 | assembly_bay_for_compact_argon_purifier, fabrication_plant_for_compact_argon_purifier, integration_works_for_compact_argon_purifier, outfitting_manufactory_for_compact_argon_purifier |  |
| `build_compact_copper_wiring_plant` | 4 | L1/L2/L3/L4 | assembly_bay_for_compact_copper_wiring_plant, fabrication_line_for_compact_copper_wiring_plant, integration_works_for_compact_copper_wiring_plant, outfitting_manufactory_for_compact_copper_wiring_plant |  |
| `build_compact_deuterium_ice_processor` | 4 | L1/L2/L3/L4 | assembly_bay_for_compact_deuterium_ice_processor, fabrication_plant_for_compact_deuterium_ice_processor, integration_works_for_compact_deuterium_ice_processor, outfitting_manufactory_for_compact_deuterium_ice_processor |  |
| `build_compact_helium_ice_processor` | 4 | L1/L2/L3/L4 | assembly_bay_for_compact_helium_ice_processor, fabrication_plant_for_compact_helium_ice_processor, integration_works_for_compact_helium_ice_processor, outfitting_manufactory_for_compact_helium_ice_processor |  |
| `build_compact_hydrogen_compressor` | 4 | L1/L2/L3/L4 | assembly_bay_for_compact_hydrogen_compressor, fabrication_plant_for_compact_hydrogen_compressor, integration_works_for_compact_hydrogen_compressor, outfitting_manufactory_for_compact_hydrogen_compressor |  |
| `build_compact_lead_smelter` | 4 | L1/L2/L3/L4 | assembly_bay_for_compact_lead_smelter, fabrication_plant_for_compact_lead_smelter, integration_works_for_compact_lead_smelter, outfitting_manufactory_for_compact_lead_smelter |  |
| `build_compact_neon_ionizer` | 4 | L1/L2/L3/L4 | assembly_bay_for_compact_neon_ionizer, fabrication_plant_for_compact_neon_ionizer, integration_works_for_compact_neon_ionizer, outfitting_manufactory_for_compact_neon_ionizer |  |
| `build_compact_nitrogen_ice_processor` | 4 | L1/L2/L3/L4 | assembly_bay_for_compact_nitrogen_ice_processor, fabrication_plant_for_compact_nitrogen_ice_processor, integration_works_for_compact_nitrogen_ice_processor, outfitting_manufactory_for_compact_nitrogen_ice_processor |  |
| `build_compact_oxygen_liquefier` | 4 | L1/L2/L3/L4 | assembly_bay_for_compact_oxygen_liquefier, fabrication_plant_for_compact_oxygen_liquefier, integration_works_for_compact_oxygen_liquefier, outfitting_manufactory_for_compact_oxygen_liquefier |  |
| `build_compact_steel_refinery` | 4 | L1/L2/L3/L4 | assembly_bay_for_compact_steel_refinery, fabrication_plant_for_compact_steel_refinery, integration_works_for_compact_steel_refinery, outfitting_manufactory_for_compact_steel_refinery |  |
| `build_compact_titanium_refinery` | 4 | L1/L2/L3/L4 | assembly_bay_for_compact_titanium_refinery, fabrication_plant_for_compact_titanium_refinery, integration_works_for_compact_titanium_refinery, outfitting_manufactory_for_compact_titanium_refinery |  |
| `build_compact_water_ice_processor` | 4 | L1/L2/L3/L4 | assembly_bay_for_compact_water_ice_processor, fabrication_plant_for_compact_water_ice_processor, integration_works_for_compact_water_ice_processor, outfitting_manufactory_for_compact_water_ice_processor |  |
| `build_compact_xenon_extractor` | 4 | L1/L2/L3/L4 | assembly_bay_for_compact_xenon_extractor, fabrication_plant_for_compact_xenon_extractor, integration_works_for_compact_xenon_extractor, outfitting_manufactory_for_compact_xenon_extractor |  |
| `build_nanofiber_internal_structure` | 4 | L1/L2/L3/L4 | weaving_bench, composite_lay_up_plant, nanofiber_composite_fabrication_complex, automated_nanofiber_works |  |
| `build_onboard_h2o2_refinery` | 4 | L1/L2/L3/L4 | electrolysis_unit_assembly_line, water_reclamation_workshop, h2o2_refinery_manufacturing_plant, automated_h2o2_refinery_works |  |
| `build_onboard_hydrogen_refinery` | 4 | L1/L2/L3/L4 | hydrogen_combustor_assembly_line, onboard_fuel_unit_workshop, hydrogen_fuel_unit_manufacturing_plant, automated_hydrogen_refinery_works |  |
| `build_remote_armor_repairer_i` | 4 | L1/L2/L3/L4 | remote_armor_repairer_i_workshop, remote_armor_repairer_i_production_lab, remote_armor_repairer_i_assembly_plant, remote_armor_repairer_i_fabrication_nexus |  |
| `build_remote_armor_repairer_ii` | 4 | L1/L2/L3/L4 | remote_armor_repairer_ii_workshop, remote_armor_repairer_ii_production_lab, remote_armor_repairer_ii_assembly_plant, remote_armor_repairer_ii_fabrication_nexus |  |
| `build_remote_armor_repairer_iii` | 4 | L1/L2/L3/L4 | remote_capital_grade_repair_integrator, remote_heavy_repair_system_fabrication_plant, remote_capital_ship_repair_manufacturing_line, remote_advanced_capital_repair_system_factory |  |
| `build_scrap_harpoon` | 1 | L1 | harpoon_rigging_shop |  |
| `build_stasis_webifier_i` | 4 | L1/L2/L3/L4 | stasis_webifier_i_workshop, stasis_webifier_i_production_lab, stasis_webifier_i_assembly_facility, stasis_webifier_i_technology_nexus |  |
| `build_stasis_webifier_ii` | 4 | L1/L2/L3/L4 | stasis_webifier_ii_workshop, stasis_webifier_ii_production_lab, stasis_webifier_ii_assembly_facility, stasis_webifier_ii_technology_nexus |  |
| `build_vanadium_ejector` | 4 | L1/L2/L3/L4 | eddy_coil_bench, separator_module_plant, eddy_current_fabrication_complex, automated_ejector_works |  |
| `build_warp_core_stabilizer` | 4 | L1/L2/L3/L4 | warp_core_stabilizer_workshop, warp_core_stabilizer_production_lab, warp_core_stabilizer_assembly_facility, warp_core_stabilizer_technology_nexus |  |
| `build_warp_disruptor` | 4 | L1/L2/L3/L4 | warp_disruptor_workshop, warp_disruptor_production_lab, warp_disruptor_assembly_facility, warp_disruptor_technology_nexus |  |
| `build_warp_scrambler` | 4 | L1/L2/L3/L4 | warp_scrambler_workshop, warp_scrambler_production_lab, warp_scrambler_assembly_facility, warp_scrambler_technology_nexus |  |
| `carapace_plating` | 2 | L1/L2 | carapace_press, carapace_lamination_works |  |
| `cast_alumina_ceramite` | 4 | L1/L2/L3/L4 | alumina_sintering_kiln, alumina_ceramite_kiln, alumina_composite_plant, corundum_composite_foundry |  |
| `caustic_polymer_synthesis` | 1 | L2 | caustic_processing_plant |  |
| `cobalt_ink_refining` | 2 | L1/L2 | cobalt_ink_leecher, cobalt_concentration_plant |  |
| `cobalt_superconductor` | 1 | L2 | cobalt_superconductor_foundry |  |
| `crystal_melt_distillation` | 1 | L1 | brine_coolant_processor |  |
| `ember_gland_combustion` | 1 | L1 | ember_gland_furnace |  |
| `exotic_distillate_refining` | 1 | L2 | nebula_distillery |  |
| `exotic_lattice_fusion` | 1 | L3 | exotic_lattice_foundry |  |
| `fabricate_gold_circuit_boards` | 4 | L1/L2/L3/L4 | gold_circuit_press, gold_film_lab, goldworks_fabricator, auric_electronics_complex |  |
| `ferrous_nodule_smelting` | 1 | L1 | slag_smeltery |  |
| `fire_lithium_ceramic` | 4 | L1/L2/L3/L4 | lithium_ceramic_kiln, lithia_kiln_array, lithium_ceramic_foundry, zerothermal_ceramic_plant |  |
| `foil_scale_smelting` | 1 | L1 | foil_scale_smeltery |  |
| `forge_trade_authenticator_pirate` | 1 | L5 | the_wire_room | pirates |
| `fuse_leaded_glass` | 4 | L1/L2/L3/L4 | leaded_glass_furnace, crystal_glass_refinery, leaded_glass_works, flux_glass_synthesis |  |
| `fusion_pearl_processing` | 1 | L2 | fusion_pearl_mill |  |
| `galvanic_cell_assembly` | 1 | L2 | galvanic_biorefinery |  |
| `gilded_chitin_smelting` | 1 | L1 | chitin_gold_furnace |  |
| `graphene_thread_weaving` | 1 | L1 | graphene_weave_reactor |  |
| `hoarfrost_core_reactor` | 1 | L2 | hoarfrost_core_forge |  |
| `ion_nodule_extraction` | 1 | L2 | ion_extraction_press |  |
| `irradiated_marrow_tap` | 1 | L2 | marrow_tap_reactor |  |
| `krypton_bladder_purification` | 1 | L1 | krypton_purification_lab |  |
| `lattice_jump_core` | 1 | L3 | lattice_jump_assembler |  |
| `leviathan_core_reactor` | 1 | L2 | leviathan_core_forge |  |
| `levity_gas_synthesis` | 1 | L1 | levity_gas_cracker |  |
| `mantis_claw_plating` | 1 | L1 | mantis_claw_press |  |
| `nitrogen_bladder_decanting` | 1 | L1 | cryo_decant_station |  |
| `noble_gas_liquefaction` | 1 | L1 | noble_gas_liquefier |  |
| `pan_galactic_alloy_synthesis` | 2 | L2/L3 | prismatic_alloy_forge, pan_galactic_foundry |  |
| `radiant_bile_enrichment` | 1 | L2 | radiant_bile_enricher |  |
| `rime_fang_plating` | 1 | L1 | rime_fang_press |  |
| `silica_lens_fabrication` | 1 | L2 | silica_optics_lab |  |
| `storm_node_capacitance` | 1 | L2 | storm_node_capacitor_bank |  |
| `superfluid_winding` | 1 | L2 | superfluid_winder |  |
| `tritium_pearl_enrichment` | 1 | L2 | tritium_pearl_enricher |  |
| `tungsten_nugget_drawing` | 1 | L2 | snail_draw_mill |  |
| `upgrade_deep_core_extractor_iii` | 4 | L1/L2/L3/L4 | extractor_refit_bay, exotic_integration_hall, deep_core_exotic_fabrication_works, automated_megacore_foundry |  |
| `vanadium_plate_forging` | 1 | L2 | scarab_plate_forge |  |
| `verdigris_smelting` | 1 | L1 | verdigris_smeltery |  |
| `whale_oil_rendering` | 1 | L1 | blubber_rendery |  |
| `(blank recipe_id)` | 3 | L1/L2/L3 | recycling_processor_mk_i, recycling_processor_mk_ii, recycling_processor_mk_iii |  |
