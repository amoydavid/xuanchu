package task

func WouldCreateDependencyCycle(graph map[string][]string, taskUUID, dependsOn string) bool {
	if taskUUID == dependsOn {
		return true
	}
	seen := map[string]bool{}
	var visit func(string) bool
	visit = func(current string) bool {
		if current == taskUUID {
			return true
		}
		if seen[current] {
			return false
		}
		seen[current] = true
		for _, next := range graph[current] {
			if visit(next) {
				return true
			}
		}
		return false
	}
	return visit(dependsOn)
}
