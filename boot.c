#define _GNU_SOURCE
#include <sched.h>
#include <stdio.h>
#include <string.h>
#include <stdbool.h>
#include <unistd.h>
#include <stdlib.h>
#include <ctype.h>
#include <fcntl.h>
#include <getopt.h>
#include <sys/stat.h>
#include <sys/mman.h>
#include <sys/mount.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <limits.h>
#include <errno.h>
#include <signal.h>

#define FUSE_USE_VERSION FUSE_MAKE_VERSION(3, 14)
#include <fuse.h>

#include "light.h"
#include "str.h"
#include "list.h"
#include "hash_table.h"
#include "atom.h"
#include "deque.h"
#include "state.h"

typedef struct mappings_chunk *Chunk;
typedef struct mappings_block *Block;
typedef struct mappings_chunk_ref *ChunkRef;

void normalize_chunk_name(const char *orig, char *norm);
Str content_of_chunk(Chunk chk);
Str expanded_content_of_block(Block blk);
void transform(Str txt, List flts);

struct mappings_chunk {
    char *chunk_name;
    List blocks;
};

struct mappings_block {
    Chunk chunk;
    List filters;
    List contents;
};

struct mappings_chunk_ref {
    Chunk chunk;
    List filters;
};
typedef struct {
    char *file;
    int start;
    int end;
} *TextRef;
typedef struct {
    enum {
        BLOCK_CONTENT_TEXT,
        BLOCK_CONTENT_CHUNK,
    } type;
    union {
        TextRef text;
        ChunkRef chunk;
    } value;
} *BlockContent;
typedef struct {
    enum {
        SECTION_COMMENT,
        SECTION_BLOCK,
    } type;
    union {
        List comment; /* A list of TextRef */
        Block block;
    } content;
} *Section;
typedef struct {
    enum {
        NODE_DIRECTORY,
        NODE_REGULAR,
        NODE_EXECUTABLE,
    } type;
    union {
        List nodes;
        Chunk chunk;
    } value;
    char *name;
} *Node;
typedef struct {
    char *file;
    FILE *in;
    /* current position, current column, current row */
    int cp, cc, cr;
    /* previous position, previous column, previous row */
    int pp, pc, pr;
} *InputFrame;

typedef struct {
    enum {
        TOKEN_CONTROL_CHARACTER,
        TOKEN_NORMAL_CHARACTER,
        TOKEN_NEWLINE,
        TOKEN_EOF,
    } type;
    char value;
} *Token;
struct state_parse_block_local {
    enum {
        CHUNK_TYPE_PLAIN_TEXT,
        CHUNK_TYPE_REGULAR_FILE,
        CHUNK_TYPE_EXECUTABLE_FILE
    } chunk_type;
    Str chunk_name;
    Block block;
};

bool to_print_help;
bool to_print_version_info;
bool to_print_specific_chunk;
bool to_execute_specific_chunk;

char *archive_name = NULL;
HashTable file_to_mmap_addr = NULL;
HashTable chunk_name_to_chunk = NULL;
List sections;
Node root_node;
List stack_of_chunks_in_tangling = empty_list;
char working_dir[PATH_MAX];
List input_frames;
State state_parse;
State state_parse_comment;
State state_parse_block;
State state_parse_block_chunk_name;
State state_parse_block_contents;
State state_parse_block_filters;
State state_parse_block_contents_text;
State state_parse_block_contents_reference;
State state_parse_block_contents_reference_name;
State state_parse_block_contents_reference_filters;
char *archive_data = NULL;
char *fuse_mount_point = NULL;
char *work_dir = NULL;
char *upper_layer = NULL;
char *mount_point = NULL;
const char *litar_data = ".litar";
int pipe_fuse_to_main[2];
pid_t fuse_pid;
char version[] = "dev";
char *name_of_chunk_to_print;
char *name_of_chunk_to_execute;

void
print_usage()
{
    printf("Usage:\tlitar [--help|--version] \n\tlitar [-p CHUNK|-x CHUNK] "
           "ARCHIVE\n");
}
Chunk
chunk_of_name(const char *name)
{
    char *norm;
    norm = strdup(name);
    normalize_chunk_name(name, norm);

    Chunk chk;
    chk = get_from_hash_table(chunk_name_to_chunk, norm);
    if (chk == NULL) {
        NEW0(chk);
        put_to_hash_table(chunk_name_to_chunk, norm, chk);
        chk->chunk_name = norm;
    } else {
        free(norm);
    }

    return chk;
}

bool
chunk_of_name_exist(const char *name)
{
    char *norm;
    norm = strdup(name);
    normalize_chunk_name(name, norm);

    Chunk chk;
    chk = get_from_hash_table(chunk_name_to_chunk, norm);

    free(norm);
    return chk != NULL;
}

const char *
name_of_chunk(Chunk chk)
{
    return chk->chunk_name;
}
List
blocks_of_chunk(Chunk chk)
{
    return chk->blocks;
}
void
block_extend_chunk(Block blk, Chunk chk)
{
    List blks = blocks_of_chunk(chk);
    blks = cons(blk, blks);
    chk->blocks = blks;
    blk->chunk = chk;
}
List
filters_of_block(Block blk)
{
    return blk->filters;
}
void
block_is_transformed_by(Block blk, Chunk flt)
{
    push(flt, &blk->filters);
}
List
contents_of_block(Block blk)
{
    return blk->contents;
}

void
block_contain_text(Block blk, TextRef tr)
{
    List bcs = contents_of_block(blk);
    BlockContent bc = NULL;

    NEW0(bc);
    bc->type = BLOCK_CONTENT_TEXT;
    bc->value.text = tr;

    bcs = cons(bc, bcs);
    blk->contents = bcs;
}

void
block_include_chunk(Block blk, ChunkRef cr)
{
    List bcs = contents_of_block(blk);
    BlockContent bc = NULL;

    NEW0(bc);
    bc->type = BLOCK_CONTENT_CHUNK;
    bc->value.chunk = cr;

    bcs = cons(bc, bcs);
    blk->contents = bcs;
}
Chunk
chunk_extended_by(Block blk)
{
    return blk->chunk;
}
List
filters_of_chunk_ref(ChunkRef ckr)
{
    return ckr->filters;
}

void
chunk_ref_reference_to(ChunkRef ckr, Chunk chk)
{
    ckr->chunk = chk;
}

void
chunk_ref_is_transformed_by(ChunkRef ckr, Chunk flt)
{
    push(flt, &ckr->filters);
}

Chunk
chunk_of_chunk_ref(ChunkRef ckr)
{
    return ckr->chunk;
}
Node
node_under_node(const char *file, Node node)
{
    Node nd;
    if (node->type != NODE_DIRECTORY) {
        fprintf(stderr, "Node %s is not directory\n", node->name);
        exit(1);
    }
    FOREACH (nd, node->value.nodes) {
        if (strcmp(file, nd->name) == 0) {
            return nd;
        }
    }
    return NULL;
}

Node
node_of_path(const char *path)
{
    char *path_t = strdup(path);
    Node node = root_node;
    int i = 0, f = i, l = strlen(path);

    if (path[0] == '/') {
        ++i;
        ++f;
    }

    while (f < l) {
        if (path_t[i] == '/' || path_t[i] == '\0') {
            path_t[i] = '\0';
            if ((node = node_under_node(path_t + f, node)) == NULL) {
                break;
            }
            f = i + 1;
        }
        ++i;
    }

    free(path_t);
    return node;
}

Node
node_of_chunk(Chunk chk)
{
    return node_of_path(name_of_chunk(chk));
}

void
chunk_is_file(Chunk chk)
{
    char *name = strdup(name_of_chunk(chk));
    Node node = root_node, nd;
    int i = 0, f = i, l = strlen(name);
    while (f < l) {
        if (name[i] == '/') {
            name[i] = '\0';
            if ((nd = node_under_node(name + f, node)) == NULL) {
                NEW(nd);
                nd->type = NODE_DIRECTORY;
                nd->name = strdup(name + f);
                nd->value.nodes = empty_list;
                push(nd, &node->value.nodes);
            }
            node = nd;
            f = i + 1;
        }
        if (name[i] == '\0') {
            if ((nd = node_under_node(name + f, node)) == NULL) {
                NEW(nd);
                nd->type = NODE_REGULAR;
                nd->name = strdup(name + f);
                nd->value.chunk = chk;
                push(nd, &node->value.nodes);
            }
            break;
        }
        ++i;
    }
    free(name);
}

void
chunk_is_executable(Chunk chk)
{
    chunk_is_file(chk);
    Node nd;

    nd = node_of_chunk(chk);
    nd->type = NODE_EXECUTABLE;
}

char *
fetch_text_from(TextRef tr)
{
    void *addr;

    addr = get_from_hash_table(file_to_mmap_addr, tr->file);
    if (addr == NULL) {
        struct stat sbuf;
        int f;

        if ((f = open(tr->file, O_RDONLY)) < 0) {
            perror(tr->file);
            exit(1);
        }

        if (fstat(f, &sbuf) < 0) {
            perror(tr->file);
            exit(1);
        }

        addr = mmap(NULL, sbuf.st_size, PROT_READ, MAP_SHARED, f, 0);
        if (addr == MAP_FAILED) {
            perror(tr->file);
            exit(1);
        }
        RESERVE(put_to_hash_table(file_to_mmap_addr, tr->file, addr));
    }

    char *str = NULL;
    int len = tr->end - tr->start;
    CALLOC(str, len + 1);
    memcpy(str, addr + tr->start, len);
    str[len] = '\0';
    return str;
}
void
normalize_chunk_name(const char *orig, char *norm)
{
    int i = 0, j = 0, l = strlen(orig);
    while (i < l) {
        if (isspace(orig[i])) {
            if (j != 0 && norm[j - 1] != ' ') {
                norm[j++] = ' ';
            }
        } else {
            norm[j++] = orig[i];
        }
        ++i;
    }
    if (norm[j - 1] == ' ') {
        norm[j - 1] = '\0';
    }
}
bool
chunk_is_in_tangling(Chunk chk)
{
    Chunk chk_t;
    FOREACH (chk_t, stack_of_chunks_in_tangling) {
        if (chk_t == chk) {
            return true;
        }
    }
    return false;
}
Str
content_of_chunk(Chunk chk)
{
    if (chunk_is_in_tangling(chk)) {
        fprintf(stderr, "Recursively include \"%s\"\n", name_of_chunk(chk));
        exit(1);
    }

    Str cc = NULL;
    List blks = copy_list(blocks_of_chunk(chk));
    Block blk;

    reverse(&blks);

    push(chk, &stack_of_chunks_in_tangling);
    new_str(&cc);
    FOREACH (blk, blks) {
        Str exb = expanded_content_of_block(blk);
        transform(exb, blk->filters);
        str_extend(cc, raw_string(exb));
        free_str(&exb);
    }
    pop(&stack_of_chunks_in_tangling);

    free_list(&blks);
    return cc;
}

Str
expanded_content_of_block(Block blk)
{
    Str ect = NULL;
    List bcs = copy_list(contents_of_block(blk));
    BlockContent bc;

    reverse(&bcs);

    new_str(&ect);
    FOREACH (bc, bcs) {
        if (bc->type == BLOCK_CONTENT_TEXT) {
            char *txt;
            txt = fetch_text_from(bc->value.text);
            str_extend(ect, txt);
            FREE(txt);
        }
        if (bc->type == BLOCK_CONTENT_CHUNK) {
            Str cc = content_of_chunk(bc->value.chunk->chunk);
            transform(cc, filters_of_chunk_ref(bc->value.chunk));
            str_extend(ect, raw_string(cc));
            free_str(&cc);
        }
    }

    free_list(&bcs);

    return ect;
}
pid_t
run_filter(Chunk flt, int in, int out)
{
    pid_t pid;
    if ((pid = fork()) < 0) {
        perror("fork");
        exit(1);
    } else if (pid == 0) {
        if (chdir(mount_point) < 0) {
            perror("chdir");
            exit(1);
        }

        if (dup2(in, STDIN_FILENO) < 0) {
            perror("dup2");
            exit(1);
        }
        close(in);

        if (dup2(out, STDOUT_FILENO) < 0) {
            perror("dup2");
            exit(1);
        }
        close(out);

        int fd;

        if ((fd = memfd_create(name_of_chunk(flt), 0)) < 0) {
            perror("memfd_create");
            exit(1);
        }

        do {
            Str ct;
            int n, wtn = 0;
            ct = content_of_chunk(flt);

            while ((n = write(fd, raw_string(ct) + wtn, str_length(ct) - wtn))
                   > 0) {
                wtn += n;
            }

            free_str(&ct);
        } while (0);

        do {
            char *name = strdup(name_of_chunk(flt));
            char *argv[2] = {name, NULL};
            if (fexecve(fd, argv, environ) < 0) {
                perror("fexecve");
                exit(1);
            }
        } while (0);
    }
    return pid;
}
void
transform(Str txt, List flts)
{
    if (flts == empty_list) {
        return;
    }

    flts = copy_list(flts);
    reverse(&flts);

    int len = length(flts);

    int in, out;

    if ((in = memfd_create("file1", 0)) < 0) {
        perror("memfd_create");
        exit(1);
    }
    do {
        int n, wtn = 0;
        while ((n = write(in, raw_string(txt) + wtn, str_length(txt) - wtn))
               > 0) {
            wtn += n;
        }
        lseek(in, 0, SEEK_SET);
    } while (0);

    if ((out = memfd_create("file2", 0)) < 0) {
        perror("memfd_create");
        exit(1);
    }

    Chunk flt;
    FOREACH (flt, flts) {
        pid_t pid;
        int st;
        lseek(in, 0, SEEK_SET);
        lseek(out, 0, SEEK_SET);
        pid = run_filter(flt, in, out);

        do {
            int t;

            t = in;
            in = out;
            out = t;
        } while (0);
        waitpid(pid, &st, 0);
        if (WIFEXITED(st)) {
            if (WEXITSTATUS(st) != 0) {
                fprintf(
                    stderr,
                    "filter @<%s@> failed, error code: %d\n",
                    name_of_chunk(flt),
                    WEXITSTATUS(st)
                );
                exit(1);
            }
        } else {
            fprintf(
                stderr,
                "filter @<%s@> failed, unknown reason",
                name_of_chunk(flt)
            );
            exit(1);
        }
    }
    free_list(&flts);

    lseek(in, 0, SEEK_SET);
    do {
        int n, rd;
        char buf[1000];

        rd = 0;

        str_clean(txt);
        while ((n = read(in, buf, 999)) > 0) {
            buf[n] = '\0';
            str_extend(txt, buf);
        }

    } while (0);

    return;
}
static Token
next_token(InputFrame ipt)
{
    Token tkn = NULL;
    char c;
    int n = 0;

    if (ipt->cp == 0) {
        if ((ipt->in = fopen(ipt->file, "r")) == NULL) {
            perror(ipt->file);
            exit(1);
        }
    }

    NEW0(tkn);

    ipt->pp = ipt->cp;
    ipt->pc = ipt->cc;
    ipt->pr = ipt->cr;

    n += (c = fgetc(ipt->in)) == EOF ? 0 : 1;
    if (c == '\100') {
        n += (c = fgetc(ipt->in)) == EOF ? 0 : 1;
        if (isalpha(c)) {
            n -= ungetc(c, ipt->in) == EOF ? 0 : 1;
            tkn->type = TOKEN_NORMAL_CHARACTER;
            tkn->value = '\100';
        } else {
            tkn->type = TOKEN_CONTROL_CHARACTER;
            if (c == EOF || c == '\n' || c == '\t' || c == ' ') {
                tkn->value = ' ';
            } else {
                tkn->value = c;
            }
            if (c == '\n') {
                n -= ungetc(c, ipt->in) == EOF ? 0 : 1;
            }
        }
    } else if (c == '\n') {
        tkn->type = TOKEN_NEWLINE;
        tkn->value = '\n';
        ipt->cp += n;
        ++ipt->cr;
        ipt->cc = 0;
        n = 0;
    } else if (c == EOF) {
        tkn->type = TOKEN_EOF;
        tkn->value = EOF;
    } else {
        tkn->type = TOKEN_NORMAL_CHARACTER;
        tkn->value = c;
    }
    ipt->cp += n;
    ipt->cc += n;

    return tkn;
}
static State
state_parse_in(State s, Signal sig)
{
    return state_parse_comment;
}

static State
state_parse_handler(State s, Signal sig)
{
    do {
        Token tkn;
        RENAME(sig, tkn);
    } while (0);

    return NULL;
}

static State
state_parse_out(State s, Signal sig)
{
    return NULL;
}
static State
state_parse_comment_in(State s, Signal sig)
{
    InputFrame ipt;
    RENAME(car(input_frames), ipt);

    TextRef txt = NULL;
    NEW0(txt);
    txt->file = ipt->file;
    txt->start = ipt->cp;
    txt->end = ipt->cp;

    List txts = empty_list;
    push(txt, &txts);

    *state_local(s) = txts;

    return NULL;
}

static State
state_parse_comment_handler(State s, Signal sig)
{
    do {
        Token tkn;
        RENAME(sig, tkn);
        if (tkn->type == TOKEN_NORMAL_CHARACTER
            || tkn->type == TOKEN_NEWLINE) {
            List txts;
            TextRef txt;
            InputFrame ipt;

            RENAME(car(input_frames), ipt);
            RENAME(*state_local(s), txts);
            RENAME(car(txts), txt);

            txt->end = ipt->cp;
            return s;
        }

        if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '\100') {
            List *txts_r;
            TextRef txt;
            InputFrame ipt;

            RENAME(car(input_frames), ipt);
            RENAME(state_local(s), txts_r);

            txt = NULL;
            NEW0(txt);
            txt->file = ipt->file;
            txt->start = ipt->pp + 1;
            txt->end = ipt->pp + 1;

            push(txt, txts_r);
            return s;
        }
        if (tkn->type == TOKEN_CONTROL_CHARACTER
            && (tkn->value == '<' || tkn->value == '[' || tkn->value == '(')) {
            return state_parse_block;
        }
    } while (0);

    return NULL;
}

static State
state_parse_comment_out(State s, Signal sig)
{
    Section sec = NULL;
    NEW0(sec);
    sec->type = SECTION_COMMENT;
    sec->content.comment = *state_local(s);
    push(sec, &sections);
    return NULL;
}
static State
state_parse_block_in(State s, Signal sig)
{
    Token tkn;
    RENAME(sig, tkn);

    struct state_parse_block_local *lc_r = NULL;
    NEW0(lc_r);
    NEW0(lc_r->block);
    new_str(&lc_r->chunk_name);

    switch (tkn->value) {
    case '<':
        lc_r->chunk_type = CHUNK_TYPE_PLAIN_TEXT;
        break;
    case '[':
        lc_r->chunk_type = CHUNK_TYPE_REGULAR_FILE;
        break;
    case '(':
        lc_r->chunk_type = CHUNK_TYPE_EXECUTABLE_FILE;
        break;
    }
    *state_local(s) = lc_r;

    return state_parse_block_chunk_name;
}

static State
state_parse_block_handler(State s, Signal sig)
{
    do {
        Token tkn;
        RENAME(sig, tkn);
        if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == ' ') {
            return state_parse_comment;
        }
    } while (0);

    return NULL;
}

static State
state_parse_block_out(State s, Signal sig)
{
    struct state_parse_block_local *lc_r;
    RENAME(*state_local(s), lc_r);

    Chunk chk;
    chk = chunk_of_name(raw_string(lc_r->chunk_name));
    block_extend_chunk(lc_r->block, chk);

    switch (lc_r->chunk_type) {
    case CHUNK_TYPE_REGULAR_FILE:
        chunk_is_file(chk);
        break;
    case CHUNK_TYPE_EXECUTABLE_FILE:
        chunk_is_executable(chk);
        break;
    default:
    }

    Section sec = NULL;
    NEW0(sec);
    sec->type = SECTION_BLOCK;
    sec->content.block = lc_r->block;
    push(sec, &sections);

    free_str(&lc_r->chunk_name);
    FREE(*state_local(s));
    return NULL;
}
static State
state_parse_block_chunk_name_in(State s, Signal sig)
{
    return NULL;
}

static State
state_parse_block_chunk_name_handler(State s, Signal sig)
{
    do {
        Token tkn;
        RENAME(sig, tkn);
        if (tkn->type == TOKEN_NORMAL_CHARACTER
            || tkn->type == TOKEN_NEWLINE) {
            struct state_parse_block_local *lc_r;
            RENAME(*state_local(state_parse_block), lc_r);

            str_append(lc_r->chunk_name, tkn->value);
            return s;
        }

        if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '\100') {
            struct state_parse_block_local *lc_r;
            RENAME(*state_local(state_parse_block), lc_r);

            str_append(lc_r->chunk_name, '\100');
            return s;
        }

        if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '=') {
            return state_parse_block_contents;
        }
    } while (0);

    return NULL;
}

static State
state_parse_block_chunk_name_out(State s, Signal sig)
{
    return NULL;
}
static State
state_parse_block_contents_in(State s, Signal sig)
{
    return state_parse_block_contents_text;
}

static State
state_parse_block_contents_handler(State s, Signal sig)
{
    do {
        Token tkn;
        RENAME(sig, tkn);
        if (state_is_active(state_parse_block_contents_text)
            && tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '|') {
            return state_parse_block_filters;
        }
    } while (0);

    return NULL;
}

static State
state_parse_block_contents_out(State s, Signal sig)
{
    return NULL;
}
static State
state_parse_block_contents_text_in(State s, Signal sig)
{
    TextRef txt = NULL;
    NEW0(txt);

    InputFrame ipt;
    RENAME(car(input_frames), ipt);

    txt->file = ipt->file;
    txt->start = ipt->cp;
    txt->end = ipt->cp;

    *state_local(s) = txt;

    return NULL;
}

static State
state_parse_block_contents_text_handler(State s, Signal sig)
{
    do {
        Token tkn;
        RENAME(sig, tkn);
        if (tkn->type == TOKEN_NORMAL_CHARACTER
            || tkn->type == TOKEN_NEWLINE) {
            TextRef txt;
            InputFrame ipt;
            struct state_parse_block_local *lc_r;

            RENAME(car(input_frames), ipt);
            RENAME(*state_local(s), txt);
            RENAME(*state_local(state_parse_block), lc_r);

            if (is_empty_list(contents_of_block(lc_r->block))
                && (txt->start == txt->end && ipt->pc != 0)
                && (tkn->type == TOKEN_NEWLINE
                    || (tkn->type == TOKEN_NORMAL_CHARACTER
                        && isspace(tkn->value)))) {
                txt->start = txt->end = txt->start + 1;
            } else {
                txt->end = ipt->cp;
            }
            return s;
        }

        if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '\100') {
            struct state_parse_block_local *lc_r;
            RENAME(*state_local(state_parse_block), lc_r);

            TextRef txt;
            RENAME(*state_local(s), txt);

            block_contain_text(lc_r->block, txt);

            txt = NULL;
            NEW0(txt);

            InputFrame ipt;
            RENAME(car(input_frames), ipt);

            txt->file = ipt->file;
            txt->start = ipt->pp + 1;
            txt->end = ipt->pp + 1;

            *state_local(s) = txt;

            return s;
        }

        if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '<') {
            return state_parse_block_contents_reference;
        }
    } while (0);

    return NULL;
}

static State
state_parse_block_contents_text_out(State s, Signal sig)
{
    struct state_parse_block_local *lc_r;
    TextRef txt;
    RENAME(*state_local(s), txt);
    RENAME(*state_local(state_parse_block), lc_r);
    block_contain_text(lc_r->block, txt);
    return NULL;
}
static State
state_parse_block_contents_reference_in(State s, Signal sig)
{
    ChunkRef ckr = NULL;
    NEW0(ckr);
    *state_local(s) = ckr;
    return state_parse_block_contents_reference_name;
}

static State
state_parse_block_contents_reference_handler(State s, Signal sig)
{
    do {
        Token tkn;
        RENAME(sig, tkn);
        if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '>') {
            return state_parse_block_contents_text;
        }
    } while (0);

    return NULL;
}

static State
state_parse_block_contents_reference_out(State s, Signal sig)
{
    struct state_parse_block_local *lc_r;
    RENAME(*state_local(state_parse_block), lc_r);
    ChunkRef ckr;
    RENAME(*state_local(s), ckr);
    block_include_chunk(lc_r->block, ckr);
    return NULL;
}
static State
state_parse_block_contents_reference_name_in(State s, Signal sig)
{
    new_str((Str *)state_local(s));
    return NULL;
}

static State
state_parse_block_contents_reference_name_handler(State s, Signal sig)
{
    do {
        Token tkn;
        RENAME(sig, tkn);
        if (tkn->type == TOKEN_NORMAL_CHARACTER
            || tkn->type == TOKEN_NEWLINE) {
            str_append(*(Str *)state_local(s), tkn->value);
            return s;
        }

        if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '\100') {
            str_append(*(Str *)state_local(s), '\100');
            return s;
        }

        if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '|') {
            return state_parse_block_contents_reference_filters;
        }
    } while (0);

    return NULL;
}

static State
state_parse_block_contents_reference_name_out(State s, Signal sig)
{
    Str txt;
    RENAME(*state_local(s), txt);
    Chunk chk;
    chk = chunk_of_name(raw_string(txt));
    ChunkRef ckr;
    RENAME(*state_local(state_parse_block_contents_reference), ckr);
    chunk_ref_reference_to(ckr, chk);
    free_str((Str *)state_local(s));
    return NULL;
}
static State
state_parse_block_contents_reference_filters_in(State s, Signal sig)
{
    new_str((Str *)state_local(s));
    return NULL;
}

static State
state_parse_block_contents_reference_filters_handler(State s, Signal sig)
{
    do {
        Token tkn;
        RENAME(sig, tkn);
        if (tkn->type == TOKEN_NORMAL_CHARACTER
            || tkn->type == TOKEN_NEWLINE) {
            str_append(*(Str *)state_local(s), tkn->value);
            return s;
        }

        if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '\100') {
            str_append(*(Str *)state_local(s), '\100');
            return s;
        }

        if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '|') {
            Str txt;
            RENAME(*state_local(s), txt);

            Chunk chk;
            chk = chunk_of_name(raw_string(txt));

            ChunkRef ckr;
            RENAME(*state_local(state_parse_block_contents_reference), ckr);

            chunk_ref_is_transformed_by(ckr, chk);

            free_str((Str *)state_local(s));
            new_str((Str *)state_local(s));
            return s;
        }
    } while (0);

    return NULL;
}

static State
state_parse_block_contents_reference_filters_out(State s, Signal sig)
{
    Str txt;
    RENAME(*state_local(s), txt);
    Chunk chk;
    chk = chunk_of_name(raw_string(txt));
    ChunkRef ckr;
    RENAME(*state_local(state_parse_block_contents_reference), ckr);
    chunk_ref_is_transformed_by(ckr, chk);
    free_str((Str *)state_local(s));
    return NULL;
}
static State
state_parse_block_filters_in(State s, Signal sig)
{
    new_str((Str *)state_local(s));
    return NULL;
}

static State
state_parse_block_filters_handler(State s, Signal sig)
{
    do {
        Token tkn;
        RENAME(sig, tkn);
        if (tkn->type == TOKEN_NORMAL_CHARACTER
            || tkn->type == TOKEN_NEWLINE) {
            str_append(*(Str *)state_local(s), tkn->value);
            return s;
        }

        if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '\100') {
            str_append(*(Str *)state_local(s), '\100');
            return s;
        }

        if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '|') {
            Str txt;
            RENAME(*state_local(s), txt);

            Chunk chk;
            chk = chunk_of_name(raw_string(txt));

            struct state_parse_block_local *lc_r;
            RENAME(state_local(state_parse_block), lc_r);

            block_is_transformed_by(lc_r->block, chk);

            free_str((Str *)state_local(s));
            new_str((Str *)state_local(s));
            return s;
        }
    } while (0);

    return NULL;
}

static State
state_parse_block_filters_out(State s, Signal sig)
{
    Str txt;
    RENAME(*state_local(s), txt);

    Chunk chk;
    chk = chunk_of_name(raw_string(txt));

    struct state_parse_block_local *lc_r;
    RENAME(*state_local(state_parse_block), lc_r);
    block_is_transformed_by(lc_r->block, chk);

    free_str((Str *)state_local(s));
    return NULL;
}
void
ensure_directory(const char *path)
{
    struct stat buf;
    if (stat(path, &buf) == -1) {
        if (errno != ENOENT) {
            perror("stat");
            exit(1);
        }
        if (mkdir(path, 0777) == -1) {
            perror("mkdir");
            exit(1);
        }
        stat(path, &buf);
    }
    if (!S_ISDIR(buf.st_mode)) {
        fprintf(stderr, "%s is not a directory\n", path);
        exit(1);
    }
}
static void *
litar_fuse_init(struct fuse_conn_info *conn, struct fuse_config *cfg)
{
    (void)conn;
    (void)cfg;
}

static int
litar_fuse_readdir(
    const char *path,
    void *buf,
    fuse_fill_dir_t filler,
    off_t offset,
    struct fuse_file_info *fi,
    enum fuse_readdir_flags flags
)
{
    (void)offset;
    (void)fi;
    (void)flags;

    Node nd;

    nd = node_of_path(path);
    if (nd == NULL) {
        return -ENOENT;
    }

    if (nd->type != NODE_DIRECTORY) {
        return -ENOTDIR;
    }

    Node cld;
    FOREACH (cld, nd->value.nodes) {
        filler(buf, cld->name, NULL, 0, 0);
    }

    return 0;
}

static int
litar_fuse_getattr(
    const char *path, struct stat *stbuf, struct fuse_file_info *fi
)
{
    (void)fi;
    int res = 0;
    memset(stbuf, 0, sizeof(struct stat));

    Node nd;

    nd = node_of_path(path);
    if (nd == NULL) {
        return -ENOENT;
    }

    switch (nd->type) {
    case NODE_DIRECTORY:
        do {
            int cnt = 2;
            Node nd_;
            FOREACH (nd_, nd->value.nodes) {
                if (nd_->type == NODE_DIRECTORY) {
                    ++cnt;
                }
            }
            stbuf->st_mode = S_IFDIR | 0755;
            stbuf->st_nlink = cnt;
        } while (0);
        break;
    default:
        do {
            Str ctn;
            ctn = content_of_chunk(nd->value.chunk);
            stbuf->st_mode = S_IFREG | 0444;
            stbuf->st_nlink = 1;
            stbuf->st_size = strlen(raw_string(ctn));
            free_str(&ctn);
        } while (0);
    }
    return res;
}

static int
litar_fuse_open(const char *path, struct fuse_file_info *fi)
{
    Node nd;
    nd = node_of_path(path);

    if (nd == NULL) {
        return -ENOENT;
    }

    if ((fi->flags & O_ACCMODE) != O_RDONLY) {
        return -EACCES;
    }

    return 0;
}

static int
litar_fuse_read(
    const char *path,
    char *buf,
    size_t size,
    off_t offset,
    struct fuse_file_info *fi
)
{
    (void)fi;

    Node nd;

    nd = node_of_path(path);
    if (nd == NULL) {
        return -ENOENT;
    }

    Str ctn;

    ctn = content_of_chunk(nd->value.chunk);

    size_t len;
    len = strlen(raw_string(ctn));

    if (offset < len) {
        if (offset + size > len) {
            size = len - offset;
        }
        memcpy(buf, raw_string(ctn) + offset, size);
    } else {
        size = 0;
    }
    free_str(&ctn);
    return size;
}

static const struct fuse_operations litar_fuse_oper = {
    .init = litar_fuse_init,
    .readdir = litar_fuse_readdir,
    .getattr = litar_fuse_getattr,
    .open = litar_fuse_open,
    .read = litar_fuse_read,
};
void
print_section(Section sec)
{
    if (sec->type == SECTION_COMMENT) {
        List lst = copy_list(sec->content.comment);
        reverse(&lst);
        TextRef txt;
        FOREACH (txt, lst) {
            printf(
                "Comment: (%s, %d, %d)\nContent:\n%s\n",
                txt->file,
                txt->start,
                txt->end,
                fetch_text_from(txt)
            );
        }
    } else {
        printf("Block: %s\n", name_of_chunk(sec->content.block->chunk));
        List lst = copy_list(contents_of_block(sec->content.block));
        reverse(&lst);
        do {
            BlockContent bc;
            FOREACH (bc, lst) {
                if (bc->type == BLOCK_CONTENT_TEXT) {
                    printf(
                        "\tText: (%s, %d, %d)\n",
                        bc->value.text->file,
                        bc->value.text->start,
                        bc->value.text->end
                    );
                } else {
                    printf(
                        "\tReference: %s",
                        name_of_chunk(bc->value.chunk->chunk)
                    );
                    List lst2 = copy_list(bc->value.chunk->filters);
                    reverse(&lst2);
                    Chunk chk;
                    FOREACH (chk, lst2) {
                        printf(" | %s", name_of_chunk(chk));
                    }
                    free_list(&lst2);
                    putchar('\n');
                }
            }
        } while (0);
        free_list(&lst);
    }
}
void
print_node(Node nd, int level)
{
    char *prefix = NULL;

    CALLOC(prefix, level + 1);
    memset(prefix, '\t', level);
    prefix[level] = '\0';
    switch (nd->type) {
    case NODE_DIRECTORY:
        printf("%s%s/\n", prefix, nd->name);

        Node sub;
        FOREACH (sub, nd->value.nodes) {
            print_node(sub, level + 1);
        }
        break;
    default:
        printf("%s%s\n", prefix, nd->name);
    }
}

int
main(int argc, char *argv[])
{
    RESERVE(do {
        new_string_hash_table(&file_to_mmap_addr);
        new_string_hash_table(&chunk_name_to_chunk);
        NEW(root_node);
        root_node->type = NODE_DIRECTORY;
        root_node->value.nodes = empty_list;
        root_node->name = strdup("");
        if (getcwd(working_dir, PATH_MAX) == NULL) {
            perror("getcwd");
            exit(1);
        }
        ensure_directory(litar_data);
    } while (0));
    KEEP(do {
        int opt, option_index;
        static struct option long_options[] = {
            {   "help",       no_argument, NULL,   0},
            {"version",       no_argument, NULL,   0},
            {  "print", required_argument, NULL, 'p'},
            {"execute", required_argument, NULL, 'x'},
        };

        while (
            (opt =
                 getopt_long(argc, argv, "p:x:", long_options, &option_index))
            != -1
        ) {
            switch (opt) {
            case 0:
                if (strcmp(long_options[option_index].name, "help") == 0) {
                    to_print_help = true;
                    break;
                }
                if (strcmp(long_options[option_index].name, "version") == 0) {
                    to_print_version_info = true;
                    break;
                }

                break;
            case 'u':
                upper_layer = optarg;
                break;
            case 'm':
                mount_point = optarg;
                break;
            case 'p':
                to_print_specific_chunk = true;
                name_of_chunk_to_print = optarg;
                break;
            case 'x':
                to_execute_specific_chunk = true;
                name_of_chunk_to_execute = optarg;
                break;

            default:
                print_usage();
                exit(1);
            }
        }

        if (optind < argc) {
            archive_name = argv[optind];

            if (access(archive_name, R_OK) == -1) {
                perror(archive_name);
                exit(1);
            }
        }
    } while (0););
    do {
        bool to_continue = true;

        KEEP(do {
            if (to_print_help) {
                print_usage();
                to_continue = false;
                break;
            }
            if (to_print_version_info) {
                printf(
                    "Version %s, built at 2026-01-16T15:39+08:00\n", version
                );

                to_continue = false;
                break;
            }
        } while (0););
        if (!to_continue) {
            break;
        }

        RESERVE(do {
            input_frames = empty_list;

            do {
                InputFrame ipt = NULL;
                NEW0(ipt);
                ipt->file = strdup(archive_name);
                push(ipt, &input_frames);
            } while (0);

            new_state(
                &state_parse, root_state, STATE_XOR, state_parse_handler
            );
            state_register_in_func(state_parse, state_parse_in);
            state_register_out_func(state_parse, state_parse_out);

            new_state(
                &state_parse_comment,
                state_parse,
                STATE_XOR,
                state_parse_comment_handler
            );
            state_register_in_func(
                state_parse_comment, state_parse_comment_in
            );
            state_register_out_func(
                state_parse_comment, state_parse_comment_out
            );

            new_state(
                &state_parse_block,
                state_parse,
                STATE_XOR,
                state_parse_block_handler
            );
            state_register_in_func(state_parse_block, state_parse_block_in);
            state_register_out_func(state_parse_block, state_parse_block_out);

            new_state(
                &state_parse_block_chunk_name,
                state_parse_block,
                STATE_XOR,
                state_parse_block_chunk_name_handler
            );
            state_register_in_func(
                state_parse_block_chunk_name, state_parse_block_chunk_name_in
            );
            state_register_out_func(
                state_parse_block_chunk_name, state_parse_block_chunk_name_out
            );

            new_state(
                &state_parse_block_contents,
                state_parse_block,
                STATE_XOR,
                state_parse_block_contents_handler
            );
            state_register_in_func(
                state_parse_block_contents, state_parse_block_contents_in
            );
            state_register_out_func(
                state_parse_block_contents, state_parse_block_contents_out
            );

            new_state(
                &state_parse_block_filters,
                state_parse_block,
                STATE_XOR,
                state_parse_block_filters_handler
            );
            state_register_in_func(
                state_parse_block_filters, state_parse_block_filters_in
            );
            state_register_out_func(
                state_parse_block_filters, state_parse_block_filters_out
            );

            new_state(
                &state_parse_block_contents_text,
                state_parse_block_contents,
                STATE_XOR,
                state_parse_block_contents_text_handler
            );
            state_register_in_func(
                state_parse_block_contents_text,
                state_parse_block_contents_text_in
            );
            state_register_out_func(
                state_parse_block_contents_text,
                state_parse_block_contents_text_out
            );

            new_state(
                &state_parse_block_contents_reference,
                state_parse_block_contents,
                STATE_XOR,
                state_parse_block_contents_reference_handler
            );
            state_register_in_func(
                state_parse_block_contents_reference,
                state_parse_block_contents_reference_in
            );
            state_register_out_func(
                state_parse_block_contents_reference,
                state_parse_block_contents_reference_out
            );

            new_state(
                &state_parse_block_contents_reference_name,
                state_parse_block_contents_reference,
                STATE_XOR,
                state_parse_block_contents_reference_name_handler
            );
            state_register_in_func(
                state_parse_block_contents_reference_name,
                state_parse_block_contents_reference_name_in
            );
            state_register_out_func(
                state_parse_block_contents_reference_name,
                state_parse_block_contents_reference_name_out
            );

            new_state(
                &state_parse_block_contents_reference_filters,
                state_parse_block_contents_reference,
                STATE_XOR,
                state_parse_block_contents_reference_filters_handler
            );
            state_register_in_func(
                state_parse_block_contents_reference_filters,
                state_parse_block_contents_reference_filters_in
            );
            state_register_out_func(
                state_parse_block_contents_reference_filters,
                state_parse_block_contents_reference_filters_out
            );

            state_init(state_parse);

            while (!is_empty_list(input_frames)) {
                Token tkn;
                InputFrame ipt;

                RENAME(car(input_frames), ipt);
                tkn = next_token(ipt);

                if (tkn->type == TOKEN_EOF) {
                    fclose(ipt->in);
                    FREE(ipt);
                    pop(&input_frames);
                } else {
                    if (!state_handle_signal(state_parse, tkn)) {
                        fprintf(
                            stderr,
                            "Unexpected token at row %d column %d of %s\n",
                            ipt->pr,
                            ipt->pc,
                            ipt->file
                        );
                        exit(1);
                    }
                }

                FREE(tkn);
            }

            state_clear(state_parse);
            free_state(&state_parse);
            free_state(&state_parse_comment);
            free_state(&state_parse_block);
            free_state(&state_parse_block_chunk_name);
            free_state(&state_parse_block_contents);
            free_state(&state_parse_block_filters);
            free_state(&state_parse_block_contents_text);
            free_state(&state_parse_block_contents_reference);
            free_state(&state_parse_block_contents_reference_name);
            free_state(&state_parse_block_contents_reference_filters);
        } while (0););

        KEEP(do {} while (0););
        if (!to_continue) {
            break;
        }

        KEEP(RESERVE(do {
                 CALLOC(
                     archive_data,
                     strlen(litar_data) + strlen(archive_name) + 2
                 );
                 strcat(archive_data, litar_data);
                 strcat(archive_data, "/");
                 strcat(archive_data, archive_name);

                 ensure_directory(archive_data);

                 CALLOC(
                     fuse_mount_point,
                     strlen(archive_data) + strlen("/fuse") + 1
                 );
                 strcat(fuse_mount_point, archive_data);
                 strcat(fuse_mount_point, "/fuse");

                 ensure_directory(fuse_mount_point);

                 CALLOC(work_dir, strlen(archive_data) + strlen("/work") + 1);
                 strcat(work_dir, archive_data);
                 strcat(work_dir, "/work");

                 ensure_directory(work_dir);
                 rmdir(work_dir); /* make sure work_dir is empty */
                 mkdir(work_dir, 0777);

                 if (mount_point == NULL) {
                     CALLOC(
                         mount_point,
                         strlen(archive_data) + strlen("/overlay") + 2
                     );
                     strcat(mount_point, archive_data);
                     strcat(mount_point, "/overlay");
                 }
                 ensure_directory(mount_point);

                 if (upper_layer == NULL) {
                     CALLOC(
                         upper_layer,
                         strlen(archive_data) + strlen("/upper") + 1
                     );
                     strcat(upper_layer, archive_data);
                     strcat(upper_layer, "/upper");
                 }
                 ensure_directory(upper_layer);
             } while (0));
             do {
                 if (pipe(pipe_fuse_to_main) < 0) {
                     perror("pipe");
                     exit(1);
                 }

                 uid_t ouid = getuid();
                 uid_t ogid = getgid();

                 if (unshare(CLONE_NEWUSER | CLONE_NEWNS) == -1) {
                     perror("unshare");
                     exit(1);
                 }

                 FILE *setgroups = fopen("/proc/self/setgroups", "w");
                 if (setgroups != NULL) {
                     fprintf(setgroups, "deny");
                 }
                 fclose(setgroups);

                 FILE *uid_map = fopen("/proc/self/uid_map", "w");
                 if (uid_map == NULL) {
                     fprintf(stderr, "failed to open /proc/self/uid_map\n");
                     exit(1);
                 }
                 fprintf(uid_map, "0 %u 1\n", ouid);
                 fclose(uid_map);

                 FILE *gid_map = fopen("/proc/self/gid_map", "w");
                 if (gid_map == NULL) {
                     fprintf(stderr, "failed to open /proc/self/gid_map\n");
                     exit(1);
                 }
                 fprintf(gid_map, "0 %u 1\n", ogid);
                 fclose(gid_map);

                 if ((fuse_pid = fork()) < 0) {
                     perror("fork");
                     exit(1);
                 } else if (fuse_pid == 0) {
                     close(pipe_fuse_to_main[0]);
                     do {
                         char msg = 0;
                         struct fuse_args args = {0, NULL, 0};
                         struct fuse *fuse_r;

                         fuse_opt_add_arg(&args, "debug");

                         fuse_r = fuse_new(
                             &args,
                             &litar_fuse_oper,
                             sizeof(litar_fuse_oper),
                             NULL
                         );
                         if (fuse_r == NULL) {
                             fprintf(stderr, "Can not create fuse\n");
                             msg = 1;
                             write(pipe_fuse_to_main[1], &msg, 1);
                             goto out1;
                         }

                         if (fuse_mount(fuse_r, fuse_mount_point) != 0) {
                             fprintf(stderr, "Can not mount fuse\n");
                             msg = 1;
                             write(pipe_fuse_to_main[1], &msg, 1);
                             goto out2;
                         }

                         write(pipe_fuse_to_main[1], &msg, 1);

                         struct fuse_session *se = fuse_get_session(fuse_r);
                         if (fuse_set_signal_handlers(se) != 0) {
                             fprintf(stderr, "Can not register handlers\n");
                             goto out3;
                         }

                         fuse_loop(fuse_r);
                         fuse_remove_signal_handlers(se);

                     out3:
                         fuse_unmount(fuse_r);

                     out2:
                         fuse_destroy(fuse_r);

                     out1:
                     } while (0);

                     exit(0);
                 }
                 close(pipe_fuse_to_main[1]);

                 char msg = 1;

                 read(pipe_fuse_to_main[0], &msg, 1);
                 if (msg != 0) {
                     fprintf(stderr, "Error when mounting fuse\n");
                     exit(1);
                 }

                 do {
                     Str overlay_opt = NULL;

                     new_str(&overlay_opt);
                     str_extend(overlay_opt, ",lowerdir=");
                     str_extend(overlay_opt, fuse_mount_point);
                     str_extend(overlay_opt, ",upperdir=");
                     str_extend(overlay_opt, upper_layer);
                     str_extend(overlay_opt, ",workdir=");
                     str_extend(overlay_opt, work_dir);

                     if (mount(
                             "overlay",
                             mount_point,
                             "overlay",
                             0,
                             raw_string(overlay_opt)
                         )
                         < 0) {
                         perror("mount");
                         exit(1);
                     }

                     free_str(&overlay_opt);
                 } while (0);
             } while (0););

        KEEP(do {
            if (to_print_specific_chunk) {
                if (!chunk_of_name_exist(name_of_chunk_to_print)) {
                    fprintf(
                        stderr, "Unknown chunk: %s\n", name_of_chunk_to_print
                    );
                    exit(1);
                }
                Str cc;
                cc = content_of_chunk(chunk_of_name(name_of_chunk_to_print));
                puts(raw_string(cc));
                free_str(&cc);

                to_continue = false;
                break;
            }
            if (to_execute_specific_chunk) {
                if (!chunk_of_name_exist(name_of_chunk_to_execute)) {
                    fprintf(
                        stderr, "Unknown chunk: %s\n", name_of_chunk_to_print
                    );
                    exit(1);
                }
                Str cc;
                puts(raw_string(cc));
                free_str(&cc);

                to_continue = false;
                break;
            }
            do {
                printf("Starting executing shell\n");
                if (execl("/usr/bin/bash", "bash", "-i", NULL) != 0) {
                    perror("execl");
                    exit(1);
                }
            } while (0);
        } while (0););
    } while (0);

    if (fuse_pid != 0) {
        kill(fuse_pid, SIGTERM);
    }

    assert_memory_safety();
    return 0;
}
