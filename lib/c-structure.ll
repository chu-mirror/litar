@- C Module

@<C module meta information@=
(
    name => 'default',
    include_flag_format => '',
    @<C module properties@>
),
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
@<exported global variables@|attach extern@>
@<exported functions@>
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

@<global variables@=
@<exported global variables@>
@

@<data types@=
@<exported data types@>
@

@<functions@=
@

@<attach extern@=@<Text Processing@/basic perl settings@>
while (<STDIN>) {
    chomp;
    if ($_ != "") {
        print "extern " . $_ . "\n";
    }
}
@
