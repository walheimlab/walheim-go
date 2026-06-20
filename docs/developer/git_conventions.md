# Git Conventions & Commit Guidelines

To maintain a clean and readable history, follow these rules when writing git commit messages.

---

## Guidelines

1. **Conventional Commits**: Use the Conventional Commits format (e.g. `feat(scope): ...`, `fix(scope): ...`).
2. **Single Scope**: Limit to a maximum of 1 scope per commit.
3. **Subject Line**: Keep the first line (subject) brief, clear, and action-oriented.
4. **Explain the "Why"**:
   - The commit description should explain the *why* of the changes rather than the *how*.
   - Explaining *how* is only appropriate if the git diff itself is highly complex or non-obvious.
5. **Be Brief**: Keep descriptions concise and overall commit messages brief.
6. **No External Trackers**: Do not mention or reference any external issue trackers (e.g., Jira, GitHub Issue numbers) in the commit messages.

---

## Git Hooks

This repository includes a pre-commit hook that automatically runs unit tests and linters before you commit, but only if Go source files (`*.go`, `go.mod`, `go.sum`) are changed.

To install the git hook:

```bash
make install-hooks
```
