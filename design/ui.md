litar is a command line tool, supports both short options and long options.

```
--help: print the usage
-p, --print EXPRESSION: evaluate EXPRESSION, and print it
```

The format of EXPRESSION follows "label1@:label2@:module name@/chunk name@|filter1@|filter2".

litar search the included libraries in this order:

1. current directory;
2. directories in $LITAR_INCLUDE, follow the format with $PATH;
3. $HOME/.litar;
4. $HOME/.local/share/litar;
5. /usr/local/share/litar;
6. /usr/share/litar.

