#!/usr/bin/perl

use v5.34;
use utf8;
use warnings;
use feature qw(signatures);
use File::Temp qw(tempfile);

my $state = 'article';
my $code_block = "";
my %blocks = ();

sub eval_line_when_article($ln) {
    if ($ln =~ /\A@<\s*(.*\S)\s*@>=\s*\Z/) {
        (my $name = $1) =~ s/\s+/ /g;
        chomp $name;
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
    my @refs = $blk =~ /@<.*@>/g;

    foreach my $ref (@refs) {
        $ref =~ /@<([^@]*).*@>/;
        chomp (my $nm = $1);
        $nm =~ s/\s+/ /g;
        my $tpp = unfold_block($nm);

        my @pps = $ref =~ /@\|[^@]*/g;

        while (@pps > 0) {
            my $pp = shift @pps;
            $pp =~ s/@\|//;
            chomp $pp;
            $pp =~ s/\s+/ /;

            my ($temp_fh, $temp_fn) = tempfile(UNLINK => 1);

            print $temp_fh unfold_block($pp);
            my $cmd = "sh $temp_fn <<'EOF'\n${tpp}EOF";
            $tpp = `$cmd`;

            close $temp_fh
        }
        $ref =~ s/\|/\\|/g;
        $blk =~ s/$ref/$tpp/;
    }

    $blocks{$name} = $blk
}

my $to_extract = shift @ARGV;
chomp $to_extract;
$to_extract =~ s/\s+/ /g;

while (<<>>) {
    if ($state eq 'article') {
        eval_line_when_article($_);
    } elsif ($state eq 'code'){
        eval_line_when_code($_);
    }
}

print unfold_block($to_extract)

