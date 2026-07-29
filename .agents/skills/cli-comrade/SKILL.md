```markdown
# cli-comrade Development Patterns

> Auto-generated skill from repository analysis

## Overview
This skill teaches you the core development patterns and workflows used in the `cli-comrade` Go codebase. You'll learn about the project's coding conventions, file organization, commit style, and how to contribute using the established workflows—especially around sensitive data redaction logic. This guide is ideal for contributors looking to make effective, consistent changes to the repository.

## Coding Conventions

### File Naming
- Use **snake_case** for all file names.
  - Example: `redact_test.go`, `token_utils.go`

### Imports
- Use **relative imports** within the project.
  - Example:
    ```go
    import "../utils"
    ```

### Exports
- Use **named exports** for functions and types.
  - Example:
    ```go
    // In redact.go
    func RedactSensitive(input string) string {
        // ...
    }
    ```

### Commit Messages
- Follow **conventional commit** style.
- Use prefixes like `fix` or `docs`.
- Keep messages concise (average ~62 characters).
  - Example:  
    ```
    fix: improve redaction for Slack tokens in logs
    ```

## Workflows

### Update Redaction Logic and Tests
**Trigger:** When someone needs to improve or fix the redaction of sensitive information (e.g., Slack tokens/URLs).  
**Command:** `/update-redaction`

1. Edit the redaction logic in `internal/redact/redact.go`.
    - Example:
      ```go
      func RedactSensitive(input string) string {
          // Improved regex or logic for token/url redaction
      }
      ```
2. Update or add tests in `internal/redact/redact_test.go`.
    - Example:
      ```go
      func TestRedactSensitive(t *testing.T) {
          input := "token: xoxb-1234567890"
          expected := "token: [REDACTED]"
          result := RedactSensitive(input)
          if result != expected {
              t.Errorf("Expected %s, got %s", expected, result)
          }
      }
      ```
3. Document the change in `CHANGELOG.md`.
    - Example:
      ```
      - Improved redaction logic for Slack tokens and URLs.
      ```

## Testing Patterns

- Test files use the pattern `*_test.go`.
- Testing framework is **unknown**, but standard Go testing is likely.
- Example test structure:
  ```go
  import "testing"

  func TestFunctionName(t *testing.T) {
      // Arrange
      // Act
      // Assert
  }
  ```

## Commands

| Command            | Purpose                                                        |
|--------------------|----------------------------------------------------------------|
| /update-redaction  | Update redaction logic and corresponding tests and changelog.  |
```
