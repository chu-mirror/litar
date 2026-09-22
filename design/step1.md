Implement the design until section "Block" in C in design/litar.md.
Store the source code under directory src, named "step1.c".

The implementation should pass following test,

```
$ litar -p hello.c examples/step1.la > hello.c
$ gcc -o hello hello.c
$ ./hello # it should print "Hello"
```

