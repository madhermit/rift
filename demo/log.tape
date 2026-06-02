# `rift log` — commit history with live diff preview. Still only.
Output docs/images/log.gif
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

Type "rift log" Enter
Sleep 2s
Down@600ms 1
Sleep 1.5s
Screenshot docs/images/log.png
Sleep 300ms
Type "q"
Sleep 500ms
