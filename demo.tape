# vhs demo of the tf wrapper — run from the repo root: vhs demo.tape
Output demo.gif

Set FontSize 15
Set Width 1100
Set Height 680
Set Padding 12
Set TypingSpeed 60ms

Hide
Type `export PATH="$PWD:$PATH" && cd demo && clear`
Enter
Show

# ---- apply: plan progress -> review tree -> approve -> live apply ----
Type "tf apply"
Enter
Sleep 4.5s

# navigate the review tree
Type "j"
Sleep 600ms
Type "j"
Sleep 600ms
Enter
Sleep 2s
Type "e"
Sleep 2.5s
Type "c"
Sleep 1.2s

# approve
Type "a"
Sleep 1.2s
Type "y"
Sleep 13s
Sleep 2s

# ---- plan with changes: collapsible diff review ----
Type "tf plan -var flavor=chocolate"
Enter
Sleep 5s
Type "e"
Sleep 3.5s
Type "q"
Sleep 2.5s

# ---- destroy: review gate with red warning ----
Type "tf destroy"
Enter
Sleep 5s
Type "a"
Sleep 1.5s
Type "y"
Sleep 6s
Sleep 2s
