BINDIR     ?= $(HOME)/.local/bin
DATADIR    ?= $(HOME)/.local/share
ZSHCOMPDIR ?= $(DATADIR)/zsh/site-functions
BASHCOMPDIR ?= $(DATADIR)/bash-completion/completions
BIN        := dgs

.PHONY: install uninstall

install:
	@mkdir -p "$(BINDIR)" "$(ZSHCOMPDIR)" "$(BASHCOMPDIR)"
	go build -o "$(BINDIR)/$(BIN)" ./cmd/dgs
	"$(BINDIR)/$(BIN)" completion zsh > "$(ZSHCOMPDIR)/_$(BIN)"
	"$(BINDIR)/$(BIN)" completion bash > "$(BASHCOMPDIR)/$(BIN)"
	@echo "installed $(BINDIR)/$(BIN)"
	@echo "installed $(ZSHCOMPDIR)/_$(BIN)"
	@echo "installed $(BASHCOMPDIR)/$(BIN)"
	@case ":$$PATH:" in *":$(BINDIR):"*) ;; *) echo "note: $(BINDIR) is not on your PATH";; esac
	@echo "note: zsh caches completions; to use the new ones now, run"
	@echo "        rm -f ~/.cache/zsh/zcompdump-* ~/.zcompdump*"
	@echo "      and open a new terminal"

uninstall:
	@for f in "$(BINDIR)/$(BIN)" "$(ZSHCOMPDIR)/_$(BIN)" "$(BASHCOMPDIR)/$(BIN)"; do \
		if [ -e "$$f" ]; then rm -f "$$f" && echo "removed $$f"; \
		else echo "$$f is not installed"; fi; \
	done
