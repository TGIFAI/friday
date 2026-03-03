package consts

const PromptPreFlush = `You are about to lose access to older parts of this conversation due to context window limits. Before that happens, please review the conversation history and persist any important information you want to remember:

- Key decisions and their reasoning
- File paths and code changes made
- Unfinished tasks or pending items
- User preferences discovered in this session

Use the file_write tool to save to memory/MEMORY.md (durable facts) or memory/daily/<today>.md (today's events).

If nothing important needs saving, respond with "FLUSH_SKIP".`

const PromptSummary = `Summarize the following conversation history concisely. Preserve:
- Key decisions and their reasoning
- Important file paths, function names, and code changes
- Task progress: what was completed, what remains
- Any errors encountered and how they were resolved
- User preferences and constraints mentioned

Format as structured notes, not a narrative. Use bullet points.
Keep the summary under 2000 tokens.`
