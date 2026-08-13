.PHONY: all

all:
	@printf '%s\n' 'PF-08 synthetic Makefile must not execute in protected governance' >&2
	@exit 97
