package devcontainer

import (
	"strings"

	"github.com/jehoctor/snadcat/internal/stacks"
)

// jetbrainsCustomizations replaces the VS Code block when the JetBrains IDE is
// selected. `backend` picks which JetBrains product opens the project;
// IntelliJ is a safe default and users edit it per project.
const jetbrainsCustomizations = "\t\"customizations\": {\n" +
	"\t\t\"jetbrains\": {\n" +
	"\t\t\t\"backend\": \"IntelliJ\",\n" +
	"\t\t\t\"plugins\": [],\n" +
	"\t\t\t\"settings\": {}\n" +
	"\t\t}\n" +
	"\t}\n"

// ApplyIDECustomizations rewrites the marker-bracketed customizations block:
//
//   - vscode:    strip the marker lines, keep the block
//   - jetbrains: replace the block with the JetBrains one
//   - anything else (none): drop the block entirely
func ApplyIDECustomizations(content, ide string) string {
	var sb strings.Builder
	inBlock := false
	for _, line := range lines(content) {
		switch {
		case strings.Contains(line, "__CUSTOMIZATIONS_START__"):
			inBlock = true
			if ide == "jetbrains" {
				sb.WriteString(jetbrainsCustomizations)
			}
			continue
		case strings.Contains(line, "__CUSTOMIZATIONS_END__"):
			inBlock = false
			continue
		}
		if !inBlock || ide == "vscode" {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// CustomizeJSON substitutes the project name and selects the IDE block.
func CustomizeJSON(content, projectName, ide string) string {
	return ApplyIDECustomizations(strings.ReplaceAll(content, "__PROJECT_NAME__", projectName), ide)
}

// StackExtensionLines renders the extension entries contributed by the given
// stacks, one per line at the template's indent, replacing the
// __STACK_EXTENSIONS__ placeholder line. Empty when no stack has one, which
// drops the placeholder line.
func StackExtensionLines(resolvedStacks []string) string {
	var sb strings.Builder
	for _, ext := range stacks.Extensions(resolvedStacks) {
		sb.WriteString("\t\t\t\t\"" + ext + "\",\n")
	}
	// ApplyLinePlaceholders re-terminates the replacement, so hand it the
	// lines without their final newline.
	return strings.TrimSuffix(sb.String(), "\n")
}

// AgentExtensionLine renders the agent's extension entry, or "" to drop the
// placeholder line when the agent contributes none.
func AgentExtensionLine(extension string) string {
	if extension == "" {
		return ""
	}
	return "\t\t\t\t\"" + extension + "\","
}
