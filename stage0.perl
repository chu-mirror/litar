#!/usr/bin/perl

use v5.34;
use utf8;
use warnings;
use feature qw(signatures);

my $state = 'article';
my $code_block = "";
my %blocks = ();

sub eval_line_when_article($ln) {
    if ($ln =~ /\A@<\s*(.*\S)\s*@>=\s*\Z/) {
        (my $name = $1) =~ s/\s+/ /g;
        $state = 'code';
        $code_block = $name;
    }
}

sub eval_line_when_code($ln) {
    if ($ln =~ /\A@\s*\Z/) {
        $state = 'article';
    } elsif ($ln =~ /\A@@(\s*)\Z/) {
        $blocks{$code_block} .= "@$1\n";
    } else {
        $blocks{$code_block} .= $ln;
    }
}

sub unfold_block($name) {
    my $blk = $blocks{$name};

    while ($blk =~ /@<\s*([^@]*\S)\s*(@\|[^@]*)*@>/) {
        my @pps = @{^CAPTURE}[1..$#{^CAPTURE}];
        (my $nm = my $_nm = $1) =~ s/\s+/ /g;
        my $tpp = unfold_block($nm);

        while (@pps > 0) {
            my $pp = shift @pps;
            $pp =~ s/@\|//;
            my $cmd = "$pp << 'EOF' \n${tpp}EOF";
            $tpp = `$cmd`;
        }

        $blk =~ s/@<\s*$_nm.*@>/$tpp/;
    }

    $blocks{$name} = $blk
}

my $to_extract = shift @ARGV;

while (<<>>) {
    if ($state eq 'article') {
        eval_line_when_article($_);
    } elsif ($state eq 'code'){
        eval_line_when_code($_);
    }
}

print unfold_block($to_extract)

