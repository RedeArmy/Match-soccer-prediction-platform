package notification

// PriorityTableForTest exposes priorityTable so that exhaustiveness tests can
// assert every known EventType has an explicit priority rather than silently
// inheriting the P2Medium default.
var PriorityTableForTest = priorityTable
