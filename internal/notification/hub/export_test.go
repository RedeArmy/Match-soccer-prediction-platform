package hub

// EvictAfterDrops exposes the default eviction threshold for black-box tests.
// Tests that construct hubs without specifying Options.EvictAfterDrops will use
// this value, which equals DefaultEvictAfterDrops.
const EvictAfterDrops = DefaultEvictAfterDrops
