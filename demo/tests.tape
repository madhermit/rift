# `rift diff --tests` — the tests lens: which specs a diff added / renamed /
# modified, with the preview scrolled to the selected test.
# Produces docs/images/tests.gif (animation) and tests.png (still).
Output docs/images/tests.gif
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
Sleep 1.8s
Type "t"
Sleep 1.6s
Down@850ms 1
Sleep 1.1s
Down@850ms 1
Sleep 1.3s
Screenshot docs/images/tests.png
Sleep 600ms
Up@700ms 2
Sleep 900ms
Type "t"
Sleep 1.1s
Type "q"
Sleep 500ms
