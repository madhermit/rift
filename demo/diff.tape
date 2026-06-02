# `rift diff` — structural diff browsing + reading mode (⇥).
# Produces docs/images/diff.gif (animation) and diff.png (still).
Output docs/images/diff.gif
Require rift

Set Shell bash
Set FontSize 22
Set FontFamily "RobotoMono Nerd Font"
Set Width 1280
Set Height 800
Set Padding 24
Set Theme "Catppuccin Mocha"

Hide
Type "cd /tmp/rift-demo && clear" Enter
Show

Type "rift diff" Enter
Sleep 2s
Down@600ms 1
Sleep 1.2s
Screenshot docs/images/diff.png
Tab
Sleep 1s
Down@250ms 8
Sleep 1.2s
Up@250ms 8
Sleep 800ms
Tab
Sleep 800ms
Type "q"
Sleep 600ms
