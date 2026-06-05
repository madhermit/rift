# `rift diff` reviewed flag — tick files off as you review them (`r`), then show
# only what's left to review (`U`) and watch the list empty out.
# Produces docs/images/reviewed.gif (animation) and reviewed.png (still).
Output docs/images/reviewed.gif
Require rift

Set Shell bash
Set FontSize 22
Set FontFamily "RobotoMono Nerd Font"
Set Width 1280
Set Height 800
Set Padding 24
Set Theme "Catppuccin Mocha"

Hide
Type "cd /tmp/rift-demo && rm -f .git/rift/reviewed.json && clear" Enter
Show

Type "rift diff" Enter
Sleep 1.8s
Down@500ms 1
Sleep 500ms
Type "r"
Sleep 900ms
Down@500ms 1
Sleep 500ms
Type "r"
Sleep 1.1s
Screenshot docs/images/reviewed.png
Type "U"
Sleep 1.5s
Type "r"
Sleep 1.6s
Type "q"
Sleep 500ms
