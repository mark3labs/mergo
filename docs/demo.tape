# mergo demo. Regenerate from the repository root with:
#
#   vhs docs/demo.tape
#
# VHS's terminal (xterm.js) has no kitty graphics, so this shows the
# half-block renderer; kitty, Ghostty, WezTerm and Konsole get full-resolution
# images.

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
Hide
Type "export XDG_CONFIG_HOME=$(mktemp -d) PS1='\[\e[38;5;141m\]❯\[\e[0m\] ' && clear"
Enter
Sleep 300ms
Show

Sleep 500ms

# Open a Markdown file: every mermaid block becomes a tab.
Type "mergo examples/showcase.md"
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

# Flip through the diagram types.
Tab
Sleep 1.5s
Tab
Sleep 1.4s
Tab
Sleep 1.4s
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
Sleep 1.4s
Tab
Sleep 1.4s
Tab
Sleep 1.4s
Tab
Sleep 1.4s
Tab
Sleep 1.8s

# Key help.
Type "?"
Sleep 2.2s
Type "?"
Sleep 500ms

Type "q"
Sleep 1s
