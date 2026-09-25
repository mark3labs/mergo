# mergo demo. Regenerate from the repository root with:
#
#   vhs docs/demo.tape
#
# VHS's terminal (xterm.js) supports sixel graphics, which mergo detects and
# uses. Stock VHS only records xterm.js's text layer though, so the images
# are missing from its recordings: record with a VHS build that also captures
# the image layer (canvas.xterm-image-layer).

Output docs/demo.gif

Require mergo

Set Shell bash
Set FontSize 12
Set Width 1400
Set Height 840
Set Padding 20
Set Framerate 30
Set TypingSpeed 55ms
Set Theme "Catppuccin Mocha"
Set WindowBar Colorful
Set BorderRadius 10
Set Margin 20
Set MarginFill "#6c5ce7"

# A throwaway config dir, so the theme picked below doesn't overwrite the
# recorder's saved theme, and the demo always starts from the default one.
# The showcase is copied to a temporary directory: the demo edits it.
Hide
Type "export XDG_CONFIG_HOME=$(mktemp -d) PS1='\[\e[38;5;141m\]❯\[\e[0m\] ' && cd $(mktemp -d) && cp ~-/examples/showcase.md . && clear"
Enter
Sleep 300ms
Show

Sleep 500ms

# Open a Markdown file: every mermaid block becomes a tab.
Type "mergo showcase.md"
Sleep 400ms
Enter
Sleep 2.5s

# Zoom in on the details, pan around, then fit again.
Type "+"
Sleep 350ms
Type "+"
Sleep 350ms
Type "+"
Sleep 900ms
Type@150ms "llll"
Sleep 400ms
Type@150ms "jj"
Sleep 900ms
Type "f"
Sleep 1.2s

# Flip through a few diagram types.
Tab
Sleep 1.5s
Tab
Sleep 1.4s
Tab
Sleep 1.4s
Tab
Sleep 1.4s
Tab
Sleep 1.6s

# Theme picker with live preview.
Type "t"
Sleep 1s
Down@750ms 4
Sleep 700ms
Enter
Sleep 1.4s

# The rest of the diagrams, in the new theme.
Tab
Sleep 1.3s
Tab
Sleep 1.3s
Tab
Sleep 1.3s
Tab
Sleep 1.3s
Tab
Sleep 1.3s
Tab
Sleep 1.3s
Tab
Sleep 1.6s

# Back to the flowchart, and edit it next to the live preview.
Tab
Sleep 1.2s
Type "e"
Sleep 1.5s
Down@40ms 20
Ctrl+E
Enter
Sleep 400ms

# An incomplete link: the error shows up as you type, and saving is refused.
Type "cache -.->|warm up| "
Sleep 1.4s
Ctrl+S
Sleep 1.8s

# Completion: node names...
Type "u"
Sleep 1.2s
Tab
Sleep 1.6s

# ...and keywords. The preview follows the edits.
Enter
Type "sty"
Sleep 1s
Tab
Type " db fill:#f38ba8,stroke:#f38ba8"
Sleep 1.6s

# Valid again: save back into the Markdown file.
Ctrl+S
Sleep 1.8s
Escape
Sleep 2s

# Key help.
Type "?"
Sleep 2.2s
Type "?"
Sleep 500ms

Type "q"
Sleep 1s
