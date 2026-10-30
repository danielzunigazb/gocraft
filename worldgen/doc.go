package worldgen

// What is exact and what is approximate, measured against the 1.8.9 worlds in the player's own
// game folder (read only; see real_test.go and full_test.go, which skip with -short).
//
// Exact (every block of every checked chunk):
//   - java.util.Random, MathHelper's sine table, Perlin/octave/simplex noise.
//   - The biome map (the whole GenLayer stack): 100% of 122,624 block columns in two worlds.
//
// Very close (97-100%):
//   - Terrain shape: 99.9% of blocks fall in the same class (air, water, lava, solid) and the
//     ground is the same height in ~98% of columns; the soil layer depth matches in ~98%.
//     The rest is decoration and what the game's chunk population did in its own order.
//   - Caves and ravines follow the game's carvers; two small details are unchecked.
//
// Decoration (ores, lakes, trees, plants, snow, ice) uses the game's random sequence, so a chunk
// whose population draws what the game's did gets the same veins: 87% of the coal of one world
// lands on the saved coal, 35% in the other (forests, where some chunks draw more than modelled).
// It stays plausible everywhere and is not identical: the game populates chunks in the order the
// player reached them, which a saved world does not record, so overlapping features cannot be
// reproduced exactly from the seed alone.
//
// Approximate or missing:
//   - Dungeons: sites are not tried (the saved worlds hold one spawner in 150 chunks and their veins
//     line up only without the draws); chests have no loot.
//   - Villages, strongholds, mineshafts, temples, monuments: not generated.
//   - Mesa clay bands, jungle mega trees and melons, huge mushrooms, acacia and dark oak shapes,
//     ice spikes, desert wells and fossils, mega taiga boulders, flower-forest flower choice:
//     simplified or left out.
//   - Water and lava placed by springs and carvers are settled blocks, not flowing.
//   - Light is computed for each chunk (sky and block light spread across its neighbours); it is
//     close to the game's, not guaranteed identical.
//
// The world has no edge. A column is a function of the seed and its coordinates only, so the
// generator answers for any chunk, at any distance, from any number of goroutines at once; the
// noise wraps at 16 million blocks as the game's does.
