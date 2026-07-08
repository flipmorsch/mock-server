# Debug-First UI: the Journal Is the Home Surface

The Web UI's home screen is the live Request Journal, not the Rule list. Editing a Rule splits the view; the traffic stream stays visible at all times. We optimized the UI for the debug loop (watch traffic → read the Match Explanation → fix or create the Rule → re-test) rather than for the authoring session, because Rules are authored occasionally but debugged against all day — the live journal with near-miss explanations is what a UI adds over hand-editing YAML.

**Rejected alternative: editor-first (the original UI).** Rule management front and center, journal as a polling side panel. Fine for bulk authoring, but the debug loop paid a mode-switch on every iteration, and "why didn't my request match?" — the question that dominates real usage — had no answer surface.

**Rejected alternative: two equal modes (Build / Observe tabs).** Each screen fully optimized for its session, but the debug loop crosses journal↔editor constantly; a tab switch inside the loop is worse than a compressed-but-live journal beside the editor.

**Consequence: the editor is permanently subordinate to the stream.** It never owns the full screen, which constrains how much form can be shown at once and rules out editor layouts that assume the whole viewport.
