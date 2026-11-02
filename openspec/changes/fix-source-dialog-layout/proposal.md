# Stabilize source dialogs

Source add/edit dialogs can expand beyond their intended width when a model name
has no wrapping opportunity. The browser's default fieldset minimum width lets
that content size the form. Sticky footer buttons also overlay expanded fields,
and shared grid fields stretch select controls beside model hints or errors.

Constrain form and control widths, wrap long content, and keep fields aligned at
the top. Give editor dialogs a dedicated scrolling body between a non-scrolling
heading and footer. Keep validation errors visible above the action buttons.
Apply the same scroll containment to read-only modal bodies and account choices.

This is a presentation-only repair. Configuration, credentials, routing and
provider requests retain their existing behavior. Ship it as PacaRoute 0.8.1.
