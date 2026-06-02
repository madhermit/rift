# `rift stage` — interactive hunk staging.
# Produces docs/images/stage.gif (animation) and stage.png (still).
Output docs/images/stage.gif
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

Type "rift stage" Enter
Sleep 2s
Screenshot docs/images/stage.png
Tab
Sleep 1s
Type "n"
Sleep 800ms
Type "s"
Sleep 1.2s
Type "q"
Sleep 600ms
