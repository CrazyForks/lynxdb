Output demo.gif

Require lynxdb
Require zsh

Set Shell "zsh"
Set FontSize 20
Set Width 1500
Set Height 860
Set Framerate 30
Set TypingSpeed 28ms
Set Padding 18
Set Margin 28
Set MarginFill "#111827"
Set BorderRadius 8
Set WindowBar Colorful
Set Theme "Catppuccin Frappe"
Set CursorBlink false
Set LoopOffset 18%

Env LYNXDB_SERVER "http://127.0.0.1:3100"

Hide
Type "ZSH_HIGHLIGHT_HIGHLIGHTERS=(main pattern)"
Enter
Type "for f in /opt/homebrew/share/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh /usr/local/share/zsh-syntax-highlighting/zsh-syntax-highlighting.zsh; do [[ -r $f ]] && source $f && break; done"
Enter
Type "ZSH_HIGHLIGHT_STYLES[command]='fg=#38bdf8,bold'"
Enter
Type "ZSH_HIGHLIGHT_STYLES[single-quoted-argument]='fg=#a6e3a1'"
Enter
Type "ZSH_HIGHLIGHT_STYLES[double-quoted-argument]='fg=#a6e3a1'"
Enter
Type "typeset -gA ZSH_HIGHLIGHT_PATTERNS"
Enter
Type "ZSH_HIGHLIGHT_PATTERNS=( '(#i)(from|parse|where|stats|sort|head|top)' 'fg=#38bdf8,bold' '((#s)|[[:space:]|(),])(#i)(by|as|or|and)((#e)|[[:space:]|(),])' 'fg=#f472b6,bold' '(#i)(count|json|combined|cef|message)' 'fg=#fde68a,bold' '>=|==|-hits|-count|\\[-1h\\]' 'fg=#f472b6,bold' )"
Enter
Type "PS1='lynxdb-demo $ '"
Enter
Type "clear"
Enter
Show

Type "lynxdb status"
Enter
Wait+Screen /healthy/
Sleep 2s

Ctrl+L
Type "lynxdb shippers"
Enter
Wait+Screen /filebeat/
Sleep 2s

Ctrl+L
Type "lynxdb indexes"
Enter
Wait+Screen /nginx-access|app-json/
Sleep 2s

Ctrl+L
Type `lynxdb query 'from app-json[-1h] | parse json from message | where level=="ERROR" or level=="FATAL" | stats count() by service, level | sort -count | head 8'`
Enter
Wait+Screen /payment-svc|order-svc|user-svc/
Sleep 3s

Ctrl+L
Type "lynxdb query 'from nginx-access[-1h] | parse combined from message | where status>=500 | top 8 uri'"
Enter
Wait+Screen /\/admin|\/index.html|\/api/
Sleep 3s

Ctrl+L
Type `lynxdb query 'from cef-security[-1h] | parse cef from message | where severity=="Critical" or severity=="High" | stats count() as hits by severity, act | sort -hits | head 8'`
Enter
Wait+Screen /Critical/
Sleep 3s

Ctrl+L
Type "lynxdb query --explain 'from nginx-access[-1h] | parse combined from message | where status>=500 | top 8 uri'"
Enter
Wait+Screen /TopK|sugar:top/
Sleep 4s

Ctrl+L
Type "lynxdb shell"
Enter
Wait+Screen /lynxdb shell/
Sleep 1s

Type `from app-json[-1h] | parse json from message | top 5 level`
Enter
Wait+Screen /FATAL|ERROR|WARN/
Sleep 2s

Ctrl+L
Type `from nginx-access[-1h] | parse combined from message | where status>=500 | stats count() by status | sort status`
Enter
Wait+Screen /500|502|503|504/
Sleep 3s

Ctrl+D
Wait+Screen /lynxdb-demo/
Sleep 1s
