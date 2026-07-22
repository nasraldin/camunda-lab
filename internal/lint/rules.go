package lint

// Rules returns a fresh copy of the deterministic rule registry.
func Rules() []Rule {
	return []Rule{
		processStartEventRule{},
		disconnectedElementRule{},
		exclusiveGatewayDefaultRule{},
		exclusiveGatewayConditionRule{},
		duplicateMessageNameRule{},
		serviceTaskRetryRule{},
		timerReachableRule{},
	}
}
