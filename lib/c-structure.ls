@- C Module

@<C module meta information@=
(
    name => 'default',
    included_flag_format => '_MODULE_%s',
    @<C module properties@>
)
@

@<C module@=
@<include flags@>
@<includes@>
@<macros@>
@<data type declarations@>
@<data types@>
@<function declarations@>
@<global variables@>
@<functions@>
@

@<C module interface@=
@<interface required includes@>
@<interface specific macros@>
@<exported macros@>
@<exported data type declarations@>
@<global declarations@>
@|wrap with included flag@

@<wrap with included flag@=@<Text Processing@/basic perl settings@>
my %meta = @<C module meta information@>;

printf("#ifndef $meta{included_flag_format}\n", uc($meta{name}));
printf("#define $meta{included_flag_format}\n", uc($meta{name}));

while (<STDIN>) {
    print;
}

print "#endif\n";
@

@<includes@=
@<interface required includes@>
@

@<macros@=
@<exported macros@>
@

@<data type declarations@=
@<exported data type declarations@>
@

@<data types@=
@<exported data types@>
@

@<function declarations@=
@<functions@>
@|extract function declarations@

@<extract function declarations@=#!/bin/sh
cproto -s
@

@<global declarations@=
@<global variables@>
@<functions@>
@|extract global declarations@

@<extract global declarations@=#!/bin/sh
cproto -v -e
@

@[helper@=
hel
@

