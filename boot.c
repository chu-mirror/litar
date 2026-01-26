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
#include <limits.h>
#include <errno.h>
#include <signal.h>

#include <sys/stat.h>
#include <sys/mman.h>
#include <sys/mount.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <sys/sendfile.h>

#define FUSE_USE_VERSION FUSE_MAKE_VERSION(3, 14)
#include <fuse.h>

#include "light.h"
#include "str.h"
#include "list.h"
#include "hash_table.h"
#include "assoc_table.h"
#include "atom.h"
#include "deque.h"
#include "state.h"
#include "fileio.h"

typedef struct mappings_module *Module;
typedef struct mappings_chunk *Chunk;
typedef struct mappings_block *Block;
typedef struct mappings_chunk_ref *ChunkRef;

const char *normalize(const char *name);
Str content_of_chunk(ChunkRef ckr);
Str expanded_content_of_block(Block blk, List lbls);
void transform(Str txt, List flts);

struct mappings_module {
    const char *name;
    HashTable chunk_name_to_chunk;
};

struct mappings_chunk {
    Module module;
    const char *name;
    AssocTable labels_to_blocks;
};

struct mappings_block {
    List filters;
    List contents;
};

struct mappings_chunk_ref {
    Chunk chunk;
    List labels;
};
typedef struct {
    const char *file;
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
        struct {
            ChunkRef chunk_ref;
            List filters;
        } chunk;
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
    const char *name;
} *Node;
typedef struct {
    const char *file;
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
struct state_parse_local {
    Module module;
    State state_before_escaping;
};
struct state_parse_block_local {
    enum {
        CHUNK_TYPE_PLAIN_TEXT,
        CHUNK_TYPE_REGULAR_FILE,
        CHUNK_TYPE_EXECUTABLE_FILE
    } chunk_type;
    ChunkRef chunk_ref;
    Block block;
};
struct state_parse_block_contents_reference_local {
    ChunkRef chunk_ref;
    List filters;
};
struct state_parse_escaping_local {
    char esc;
    const char *argument;
};

bool to_print_help;
bool to_print_version_info;

int chosen_usage = -1;
char *archive_name = NULL;
HashTable file_to_mmap_addr = NULL;
HashTable module_name_to_module = NULL;
List sections;
Node root_node;
List stack_of_chunks_in_tangling = empty_list;
char working_dir[PATH_MAX];
List input_frames;
State state_parse;
State state_parse_comment;
State state_parse_block;
State state_parse_block_chunk_ref;
State state_parse_block_contents;
State state_parse_block_filters;
State state_parse_block_chunk_ref_name;
State state_parse_block_contents_text;
State state_parse_block_contents_reference;
State state_parse_block_contents_reference_chunk_ref;
State state_parse_block_contents_reference_filters;
State state_parse_block_contents_reference_chunk_ref_name;
State state_parse_block_contents_reference_filters_name;
State state_parse_block_filters_name;
State state_parse_escaping;
State state_parse_escaping_argument;
char *archive_data = NULL;
char *fuse_mount_point = NULL;
char *work_dir = NULL;
char *upper_layer = NULL;
char *mount_point = NULL;
const char *litar_data = ".litar";
int pipe_fuse_to_main[2];
pid_t fuse_pid;
char version[] = "dev";
char *mirror_point = NULL;
char *name_of_chunk_to_print;
char *name_of_chunk_to_execute;

void
print_usage()
{
    printf("Usage:\tlitar [--help|--version] \n\tlitar [-m DIR|-p CHUNK|-x "
           "CHUNK] ARCHIVE\n");
}
Module
module_of_name(const char *name)
{
    const char *norm;
    norm = normalize(name);

    Module mod;
    mod = get_from_hash_table(module_name_to_module, norm);
    if (mod == NULL) {
        RESERVE(NEW0(mod); put_to_hash_table(module_name_to_module, norm, mod);
                mod->name = norm;);
    }

    return mod;
}
Chunk
chunk_in_module_of_name(Module mod, const char *name)
{
    const char *norm;
    norm = normalize(name);

    if (!mod->chunk_name_to_chunk) {
        RESERVE(new_string_hash_table(&mod->chunk_name_to_chunk));
    }

    Chunk chk;
    chk = get_from_hash_table(mod->chunk_name_to_chunk, norm);
    if (chk == NULL) {
        RESERVE(NEW0(chk);
                put_to_hash_table(mod->chunk_name_to_chunk, norm, chk);
                chk->name = norm;
                chk->module = mod;);
    }

    return chk;
}

bool
chunk_of_name_exist_in_module(const char *name, Module mod)
{
    if (!mod->chunk_name_to_chunk) {
        return false;
    }

    const char *norm;
    norm = normalize(name);

    Chunk chk;
    chk = get_from_hash_table(mod->chunk_name_to_chunk, norm);

    return chk != NULL;
}
static int
equal_func_labels(const void *lbls1, const void *lbls2)
{
    List l1, l2;
    int r = 1;

    RENAME(lbls1, l1);
    RENAME(lbls2, l2);

    while (!is_empty_list(l1) && !is_empty_list(l2)) {
        r = r && (car(l1) == car(l2));
        if (!r) {
            break;
        }
        l1 = cdr(l1);
        l2 = cdr(l2);
    }
    if (!is_empty_list(l1) || !is_empty_list(l2)) {
        r = 0;
    }
    return r;
}

List
blocks_of_chunk_ref(ChunkRef ckr)
{
    Chunk chk = ckr->chunk;
    List lbls = ckr->labels;

    if (!chk->labels_to_blocks) {
        return empty_list;
    }

    List blks = get_from_assoc_table(chk->labels_to_blocks, lbls);

    return blks;
}

void
block_extend_chunk_ref(Block blk, ChunkRef ckr)
{
    Chunk chk = ckr->chunk;
    List lbls = ckr->labels;
    if (!chk->labels_to_blocks) {
        new_assoc_table(&chk->labels_to_blocks, equal_func_labels);
    }
    List blks = blocks_of_chunk_ref(ckr);
    blks = cons(blk, blks);
    put_to_assoc_table(chk->labels_to_blocks, lbls, blks);
}
void
block_is_transformed_by(Block blk, Chunk flt)
{
    push(flt, &blk->filters);
}
void
block_contain_text(Block blk, TextRef tr)
{
    List bcs = blk->contents;
    BlockContent bc = NULL;

    NEW0(bc);
    bc->type = BLOCK_CONTENT_TEXT;
    bc->value.text = tr;

    bcs = cons(bc, bcs);
    blk->contents = bcs;
}

void
block_include_chunk_ref_with_filters(Block blk, ChunkRef ckr, List flts)
{
    List bcs = blk->contents;
    BlockContent bc = NULL;

    NEW0(bc);
    bc->type = BLOCK_CONTENT_CHUNK;
    bc->value.chunk.chunk_ref = ckr;
    bc->value.chunk.filters = flts;

    bcs = cons(bc, bcs);
    blk->contents = bcs;
}
void
chunk_ref_is_specialized_by(ChunkRef ckr, const char *lbl)
{
    const char *norm = normalize(lbl);
    push((void *)norm, &ckr->labels);
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
    char *path_t = NULL;
    Node node = root_node;
    int i = 0, f = i, l = strlen(path);

    STRDUP(path, path_t);

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

    FREE(path_t);
    return node;
}

char *
path_of_chunk(Chunk chk)
{
    Str path = NULL;

    new_str(&path);
    if (chk->module != module_of_name("")) {
        str_extend(path, chk->module->name);
        str_extend(path, "/");
    }
    str_extend(path, chk->name);

    char *pt = NULL;
    STRDUP(raw_string(path), pt);

    free_str(&path);
    return pt;
}

Node
node_of_chunk(Chunk chk)
{
    char *path = path_of_chunk(chk);
    Node nd = node_of_path(path);

    FREE(path);

    return nd;
}

void
chunk_is_file(Chunk chk)
{
    char *path;
    Node node = root_node, nd;
    int i = 0, f = i, l;

    path = path_of_chunk(chk);
    l = strlen(path);

    while (f < l) {
        if (path[i] == '/') {
            path[i] = '\0';
            if ((nd = node_under_node(path + f, node)) == NULL) {
                NEW(nd);
                nd->type = NODE_DIRECTORY;
                nd->name = atom_str(path + f);
                nd->value.nodes = empty_list;
                push(nd, &node->value.nodes);
            }
            node = nd;
            f = i + 1;
        } else if (path[i] == '\0') {
            if ((nd = node_under_node(path + f, node)) == NULL) {
                NEW(nd);
                nd->type = NODE_REGULAR;
                nd->name = atom_str(path + f);
                nd->value.chunk = chk;
                push(nd, &node->value.nodes);
            }
            break;
        }
        ++i;
    }
    FREE(path);
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
const char *
normalize(const char *name)
{
    int i = 0, j = 0, l = strlen(name);
    char *norm = NULL;

    STRDUP(name, norm);

    l = strlen(name);
    while (i < l) {
        if (isspace(name[i])) {
            if (j != 0 && !isspace(norm[j - 1])) {
                norm[j++] = ' ';
            }
        } else {
            norm[j++] = name[i];
        }
        ++i;
    }
    if (isspace(norm[j - 1])) {
        norm[j - 1] = '\0';
    } else {
        norm[j] = '\0';
    }

    const char *a;
    RESERVE(a = atom_str(norm));

    FREE(norm);

    return a;
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
content_of_chunk(ChunkRef ckr)
{
    Chunk chk = ckr->chunk;
    if (chunk_is_in_tangling(chk)) {
        fprintf(stderr, "Recursively include \"%s\"\n", chk->name);
        exit(1);
    }

    Str cc = NULL;
    new_str(&cc);

    List remains = ckr->labels, passeds = empty_list;

    do {
        List blks;
        ChunkRef ckr = NULL;
        NEW0(ckr);

        reverse(&passeds);
        ckr->chunk = chk;
        ckr->labels = passeds;
        blks = copy_list(blocks_of_chunk_ref(ckr));
        reverse(&passeds);
        FREE(ckr);

        reverse(&blks);

        Block blk;
        push(chk, &stack_of_chunks_in_tangling);
        FOREACH (blk, blks) {
            Str exb = expanded_content_of_block(blk, remains);
            transform(exb, blk->filters);
            str_extend(cc, raw_string(exb));
            free_str(&exb);
        }
        pop(&stack_of_chunks_in_tangling);

        free_list(&blks);
        if (is_empty_list(remains)) {
            break;
        }
        push(car(remains), &passeds);
        remains = cdr(remains);
    } while (true);

    free_list(&passeds);

    return cc;
}

Str
expanded_content_of_block(Block blk, List lbls)
{
    Str ect = NULL;
    List bcs = copy_list(blk->contents);
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
            List l1, l2, l;

            l1 = copy_list(lbls);
            l2 = copy_list(bc->value.chunk.chunk_ref->labels);
            l = append(&l1, &l2);

            ChunkRef ckr = NULL;
            MOVE(bc->value.chunk.chunk_ref, ckr);
            ckr->labels = l;
            Str cc = content_of_chunk(ckr);
            FREE(ckr);

            transform(cc, bc->value.chunk.filters);
            str_extend(ect, raw_string(cc));

            free_str(&cc);
            free_list(&l);
        }
    }

    free_list(&bcs);

    return ect;
}
pid_t
run_filter(ChunkRef flt, int in, int out)
{
    pid_t pid;
    if ((pid = fork()) < 0) {
        perror("fork");
        exit(1);
    } else if (pid == 0) {
        if (in != STDIN_FILENO) {
            if (dup2(in, STDIN_FILENO) < 0) {
                perror("dup2");
                exit(1);
            }
            close(in);
        }

        if (out != STDOUT_FILENO) {
            if (dup2(out, STDOUT_FILENO) < 0) {
                perror("dup2");
                exit(1);
            }
            close(out);
        }

        int fd;

        if ((fd = memfd_create(flt->chunk->name, 0)) < 0) {
            perror("memfd_create");
            exit(1);
        }

        do {
            Str ct;
            int n, wtn = 0;
            ct = content_of_chunk(flt);
            fileio_write_str(fd, ct);
            free_str(&ct);
        } while (0);

        do {
            char *name = strdup(flt->chunk->name);
            char *argv[2] = {name, NULL};

            if (chdir(mount_point) < 0) {
                perror("chdir");
                exit(1);
            }

            if (flt->chunk->module != module_of_name("")) {
                if (chdir(flt->chunk->module->name) < 0) {
                    perror("chdir");
                    exit(1);
                }
            }

            if (fexecve(fd, argv, environ) < 0) {
                perror("fexecve");
                exit(1);
            }
        } while (0);

        close(fd);
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

    int in, out;

    if ((in = memfd_create("file1", 0)) < 0) {
        perror("memfd_create");
        exit(1);
    }
    fileio_write_str(in, txt);

    if ((out = memfd_create("file2", 0)) < 0) {
        perror("memfd_create");
        exit(1);
    }

    ChunkRef flt;
    FOREACH (flt, flts) {
        pid_t pid;
        int st;
        lseek(in, 0, SEEK_SET);
        lseek(out, 0, SEEK_SET);
        ftruncate(out, 0);
        pid = run_filter(flt, in, out);

        SWAP(in, out);

        waitpid(pid, &st, 0);
        if (WIFEXITED(st)) {
            if (WEXITSTATUS(st) != 0) {
                fprintf(
                    stderr,
                    "filter @<%s@> failed, error code: %d\n",
                    flt->chunk->name,
                    WEXITSTATUS(st)
                );
                exit(1);
            }
        } else {
            fprintf(
                stderr,
                "filter @<%s@> failed, unknown reason",
                flt->chunk->name
            );
            exit(1);
        }
    }

    lseek(in, 0, SEEK_SET);
    str_clean(txt);
    fileio_read_str(in, txt);

    close(in);
    close(out);
    free_list(&flts);

    return;
}
static Token
next_token(InputFrame ipt)
{
    Token tkn = NULL;
    char c;
    int n = 0;

    if (!ipt->in) {
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
bool
extract_name(State s, Token tkn)
{
    Str cache;
    RENAME(*state_local(s), cache);
    if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '\100') {
        str_append(cache, '\100');
        return true;
    }
    if (tkn->type == TOKEN_NORMAL_CHARACTER || tkn->type == TOKEN_NEWLINE) {
        str_append(cache, tkn->value);
        return true;
    }
    return false;
}

bool
extract_text(State s, Token tkn)
{
    List cache;
    RENAME(*state_local(s), cache);

    InputFrame ipt;
    RENAME(car(input_frames), ipt);

    if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '\100') {
        TextRef txt = NULL;

        NEW0(txt);
        txt->file = ipt->file;
        txt->start = ipt->cp - 1;
        txt->end = ipt->cp - 1;

        push(txt, &cache);
        *state_local(s) = cache;

        return true;
    }
    if (tkn->type == TOKEN_NORMAL_CHARACTER || tkn->type == TOKEN_NEWLINE) {
        TextRef txt;

        if (is_empty_list(cache)) {
            txt = NULL;

            NEW0(txt);
            txt->file = ipt->file;
            txt->start = ipt->pp;
            txt->end = ipt->pp;

            push(txt, &cache);
            *state_local(s) = cache;
        }

        RENAME(car(cache), txt);
        txt->end = ipt->cp;

        return true;
    }
    return false;
}
bool
extract_chunk_ref(State s, State sn, Token tkn)
{
    static Module mod;

    ChunkRef ckr;
    RENAME(*state_local(s), ckr);

    Str cache;
    RENAME(*state_local(sn), cache);

    if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == ':') {
        push((void *)normalize(raw_string(cache)), &ckr->labels);
        str_clean(cache);
        return true;
    }

    if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '/') {
        mod = module_of_name(normalize(raw_string(cache)));
        str_clean(cache);
        return true;
    }

    if (tkn->type == TOKEN_CONTROL_CHARACTER
        && (tkn->value == '>' || tkn->value == '|' || tkn->value == '='
            || tkn->value == ' ')) {
        if (mod == module_of_name("") || mod == NULL) {
            mod = ((struct state_parse_local *)*state_local(state_parse))
                      ->module;
        }
        ckr->chunk =
            chunk_in_module_of_name(mod, normalize(raw_string(cache)));
        mod = module_of_name("");
        str_clean(cache);
        return false;
    }

    return false;
}
static State
state_parse_in(State s, Signal sig)
{
    struct state_parse_local *lc_r = NULL;
    NEW0(lc_r);
    lc_r->module = module_of_name("");
    *state_local(s) = lc_r;
    return state_parse_comment;
}

static State
state_parse_handler(State s, Signal sig)
{
    struct state_parse_local *lc_r;
    RENAME(*state_local(s), lc_r);
    do {
        Token tkn;
        RENAME(sig, tkn);
        InputFrame ipt;
        RENAME(car(input_frames), ipt);
    } while (0);

    return NULL;
}

static State
state_parse_out(State s, Signal sig)
{
    struct state_parse_local *lc_r;
    RENAME(*state_local(s), lc_r);
    FREE(*state_local(s));
    return NULL;
}
static State
state_parse_comment_in(State s, Signal sig)
{
    List cache = empty_list;
    *state_local(s) = cache;
    return NULL;
}

static State
state_parse_comment_handler(State s, Signal sig)
{
    Token tkn;
    RENAME(sig, tkn);
    InputFrame ipt;
    RENAME(car(input_frames), ipt);
    if (ipt->cc == 2 && tkn->type == TOKEN_CONTROL_CHARACTER
        && tkn->value == '-') {
        struct state_parse_local *lc_r;
        RENAME(*state_local(state_parse), lc_r);
        lc_r->state_before_escaping = s;
        return state_parse_escaping;
    }
    if (ipt->cc == 2 && tkn->type == TOKEN_CONTROL_CHARACTER
        && tkn->value == '.') {
        struct state_parse_local *lc_r;
        RENAME(*state_local(state_parse), lc_r);
        lc_r->state_before_escaping = s;
        return state_parse_escaping;
    }

    if (extract_text(s, tkn)) {
        return s;
    }
    if (tkn->type == TOKEN_CONTROL_CHARACTER
        && (tkn->value == '<' || tkn->value == '[' || tkn->value == '(')) {
        return state_parse_block;
    }
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

    return state_parse_block_chunk_ref;
}

static State
state_parse_block_handler(State s, Signal sig)
{
    Token tkn;
    RENAME(sig, tkn);
    if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == ' ') {
        if (state_is_active(state_parse_block_filters)) {
            extract_chunk_ref(
                state_parse_block_filters, state_parse_block_filters_name, tkn
            );
        }
        return state_parse_comment;
    }
    return NULL;
}

static State
state_parse_block_out(State s, Signal sig)
{
    struct state_parse_block_local *lc_r;
    struct state_parse_local *plc_r;
    RENAME(*state_local(s), lc_r);
    RENAME(*state_local(state_parse), plc_r);

    switch (lc_r->chunk_type) {
    case CHUNK_TYPE_REGULAR_FILE:
        chunk_is_file(lc_r->chunk_ref->chunk);
        break;
    case CHUNK_TYPE_EXECUTABLE_FILE:
        chunk_is_executable(lc_r->chunk_ref->chunk);
        break;
    default:
    }

    block_extend_chunk_ref(lc_r->block, lc_r->chunk_ref);

    Section sec = NULL;
    NEW0(sec);
    sec->type = SECTION_BLOCK;
    sec->content.block = lc_r->block;
    push(sec, &sections);

    FREE(*state_local(s));
    return NULL;
}
static State
state_parse_block_chunk_ref_in(State s, Signal sig)
{
    NEW0(*(ChunkRef *)state_local(s));
    return state_parse_block_chunk_ref_name;
}

static State
state_parse_block_chunk_ref_handler(State s, Signal sig)
{
    Token tkn;
    RENAME(sig, tkn);

    if (extract_chunk_ref(s, state_parse_block_chunk_ref_name, tkn)) {
        return s;
    }
    if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '=') {
        return state_parse_block_contents;
    }
    return NULL;
}

static State
state_parse_block_chunk_ref_out(State s, Signal sig)
{
    struct state_parse_block_local *lc_r;
    RENAME(*state_local(state_parse_block), lc_r);

    lc_r->chunk_ref = (ChunkRef)*state_local(s);
    *state_local(s) = NULL;
    return NULL;
}

static State
state_parse_block_chunk_ref_name_in(State s, Signal sig)
{
    new_str((Str *)state_local(s));
    return NULL;
}

static State
state_parse_block_chunk_ref_name_handler(State s, Signal sig)
{
    Token tkn;
    RENAME(sig, tkn);
    if (extract_name(s, tkn)) {
        return s;
    }
    return NULL;
}

static State
state_parse_block_chunk_ref_name_out(State s, Signal sig)
{
    free_str((Str *)state_local(s));
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
    Token tkn;
    RENAME(sig, tkn);
    if (state_is_active(state_parse_block_contents_text)
        && tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '|') {
        return state_parse_block_filters;
    }
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
    List cache = empty_list;
    *state_local(s) = cache;
    return NULL;
}

static State
state_parse_block_contents_text_handler(State s, Signal sig)
{
    Token tkn;
    RENAME(sig, tkn);
    struct state_parse_block_local *lc_r;
    RENAME(*state_local(state_parse_block), lc_r);

    List cache;
    RENAME(*state_local(s), cache);

    if (is_empty_list(lc_r->block->contents) && is_empty_list(cache)
        && (tkn->type == TOKEN_NEWLINE
            || (tkn->type == TOKEN_NORMAL_CHARACTER && isspace(tkn->value)))) {
        return s;
    }

    if (extract_text(s, tkn)) {
        return s;
    }

    if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '<') {
        return state_parse_block_contents_reference;
    }
    return NULL;
}

static State
state_parse_block_contents_text_out(State s, Signal sig)
{
    List txts;
    RENAME(*state_local(s), txts);
    reverse(&txts);

    struct state_parse_block_local *lc_r;
    RENAME(*state_local(state_parse_block), lc_r);

    TextRef txt;
    FOREACH (txt, txts) {
        block_contain_text(lc_r->block, txt);
    }
    free_list(&txts);
    return NULL;
}
static State
state_parse_block_contents_reference_in(State s, Signal sig)
{
    struct state_parse_block_contents_reference_local *lc_r = NULL;
    NEW0(lc_r);
    *state_local(s) = lc_r;
    return state_parse_block_contents_reference_chunk_ref;
}

static State
state_parse_block_contents_reference_handler(State s, Signal sig)
{
    Token tkn;
    RENAME(sig, tkn);
    if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '>') {
        if (state_is_active(state_parse_block_contents_reference_chunk_ref)) {
            extract_chunk_ref(
                state_parse_block_contents_reference_chunk_ref,
                state_parse_block_contents_reference_chunk_ref_name,
                tkn
            );
        }
        if (state_is_active(state_parse_block_contents_reference_filters)) {
            extract_chunk_ref(
                state_parse_block_contents_reference_filters,
                state_parse_block_contents_reference_filters_name,
                tkn
            );
        }
        return state_parse_block_contents_text;
    }
    return NULL;
}

static State
state_parse_block_contents_reference_out(State s, Signal sig)
{
    struct state_parse_block_local *plc_r;
    RENAME(*state_local(state_parse_block), plc_r);

    struct state_parse_block_contents_reference_local *lc_r;
    RENAME(*state_local(s), lc_r);

    block_include_chunk_ref_with_filters(
        plc_r->block, lc_r->chunk_ref, lc_r->filters
    );
    FREE(lc_r);
    return NULL;
}
static State
state_parse_block_contents_reference_chunk_ref_in(State s, Signal sig)
{
    NEW0(*(ChunkRef *)state_local(s));
    return state_parse_block_contents_reference_chunk_ref_name;
}

static State
state_parse_block_contents_reference_chunk_ref_handler(State s, Signal sig)
{
    Token tkn;
    RENAME(sig, tkn);
    if (extract_chunk_ref(
            s, state_parse_block_contents_reference_chunk_ref_name, tkn
        )) {
        return s;
    }
    if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '|') {
        return state_parse_block_contents_reference_filters;
    }
    return NULL;
}

static State
state_parse_block_contents_reference_chunk_ref_out(State s, Signal sig)
{
    Token tkn;
    RENAME(sig, tkn);

    struct state_parse_block_contents_reference_local *lc_r;
    RENAME(*state_local(state_parse_block_contents_reference), lc_r);
    lc_r->chunk_ref = (ChunkRef)*state_local(s);
    *state_local(s) = NULL;
    return NULL;
}

static State
state_parse_block_contents_reference_chunk_ref_name_in(State s, Signal sig)
{
    new_str((Str *)state_local(s));
    return NULL;
}

static State
state_parse_block_contents_reference_chunk_ref_name_handler(
    State s, Signal sig
)
{
    Token tkn;
    RENAME(sig, tkn);

    if (extract_name(s, tkn)) {
        return s;
    }

    return NULL;
}

static State
state_parse_block_contents_reference_chunk_ref_name_out(State s, Signal sig)
{
    free_str((Str *)state_local(s));
    return NULL;
}
static State
state_parse_block_contents_reference_filters_in(State s, Signal sig)
{
    NEW0(*(ChunkRef *)state_local(s));
    return state_parse_block_contents_reference_filters_name;
}

static State
state_parse_block_contents_reference_filters_handler(State s, Signal sig)
{
    Token tkn;
    RENAME(sig, tkn);

    if (extract_chunk_ref(
            s, state_parse_block_contents_reference_filters_name, tkn
        )) {
        return s;
    }

    if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '|') {
        struct state_parse_block_contents_reference_local *lc_r;
        RENAME(*state_local(state_parse_block_contents_reference), lc_r);

        push(*state_local(s), &lc_r->filters);

        *state_local(s) = NULL;
        NEW0(*(ChunkRef *)state_local(s));
        return s;
    }
    return NULL;
}

static State
state_parse_block_contents_reference_filters_out(State s, Signal sig)
{
    Token tkn;
    RENAME(sig, tkn);

    struct state_parse_block_contents_reference_local *lc_r;
    RENAME(*state_local(state_parse_block_contents_reference), lc_r);

    push(*state_local(s), &lc_r->filters);

    *state_local(s) = NULL;
    return NULL;
}

static State
state_parse_block_contents_reference_filters_name_in(State s, Signal sig)
{
    new_str((Str *)state_local(s));
    return NULL;
}

static State
state_parse_block_contents_reference_filters_name_handler(State s, Signal sig)
{
    Token tkn;
    RENAME(sig, tkn);

    if (extract_name(s, tkn)) {
        return s;
    }

    return NULL;
}

static State
state_parse_block_contents_reference_filters_name_out(State s, Signal sig)
{
    free_str((Str *)state_local(s));
    return NULL;
}
static State
state_parse_block_filters_in(State s, Signal sig)
{
    NEW0(*(ChunkRef *)state_local(s));
    return state_parse_block_filters_name;
}

static State
state_parse_block_filters_handler(State s, Signal sig)
{
    Token tkn;
    RENAME(sig, tkn);

    if (extract_chunk_ref(s, state_parse_block_filters_name, tkn)) {
        return s;
    }

    if (tkn->type == TOKEN_CONTROL_CHARACTER && tkn->value == '|') {
        struct state_parse_block_local *lc_r;
        RENAME(*state_local(state_parse_block), lc_r);

        push(*state_local(s), &lc_r->block->filters);

        *state_local(s) = NULL;
        NEW0(*(ChunkRef *)state_local(s));
        return s;
    }
    return NULL;
}

static State
state_parse_block_filters_out(State s, Signal sig)
{
    Token tkn;
    RENAME(sig, tkn);

    struct state_parse_block_local *lc_r;
    RENAME(*state_local(state_parse_block), lc_r);

    push(*state_local(s), &lc_r->block->filters);

    *state_local(s) = NULL;
    return NULL;
}

static State
state_parse_block_filters_name_in(State s, Signal sig)
{
    new_str((Str *)state_local(s));
    return NULL;
}

static State
state_parse_block_filters_name_handler(State s, Signal sig)
{
    Token tkn;
    RENAME(sig, tkn);

    if (extract_name(s, tkn)) {
        return s;
    }

    return NULL;
}

static State
state_parse_block_filters_name_out(State s, Signal sig)
{
    free_str((Str *)state_local(s));
    return NULL;
}
static State
state_parse_escaping_in(State s, Signal sig)
{
    struct state_parse_escaping_local *lc_r = NULL;

    NEW0(lc_r);
    lc_r->esc = ((Token)sig)->value;

    *state_local(s) = lc_r;
    return state_parse_escaping_argument;
}

static State
state_parse_escaping_handler(State s, Signal sig)
{
    do {
        Token tkn;
        RENAME(sig, tkn);
        InputFrame ipt;
        RENAME(car(input_frames), ipt);
        if (tkn->type == TOKEN_NEWLINE) {
            struct state_parse_local *lc_r;
            RENAME(*state_local(state_parse), lc_r);
            return lc_r->state_before_escaping;
        }
    } while (0);

    return NULL;
}

static State
state_parse_escaping_out(State s, Signal sig)
{
    struct state_parse_escaping_local *lc_r;
    RENAME(*state_local(s), lc_r);

    if (lc_r->esc == '-') {
        struct state_parse_local *plc_r;
        RENAME(*state_local(state_parse), plc_r);
        plc_r->module = module_of_name(lc_r->argument);
    }
    if (lc_r->esc == '.') {
        InputFrame ipt = NULL;
        NEW0(ipt);
        ipt->file = normalize(lc_r->argument);
        push(ipt, &input_frames);
    }

    FREE(*state_local(s));
    return NULL;
}

static State
state_parse_escaping_argument_in(State s, Signal sig)
{
    new_str((Str *)state_local(s));
    return NULL;
}

static State
state_parse_escaping_argument_handler(State s, Signal sig)
{
    do {
        Token tkn;
        RENAME(sig, tkn);
        InputFrame ipt;
        RENAME(car(input_frames), ipt);
        if (extract_name(s, tkn)) {
            return s;
        }
    } while (0);

    return NULL;
}

static State
state_parse_escaping_argument_out(State s, Signal sig)
{
    struct state_parse_escaping_local *lc_r;
    RENAME(*state_local(state_parse_escaping), lc_r);
    lc_r->argument = normalize(raw_string((Str)*state_local(s)));
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
            ChunkRef ckr = NULL;
            NEW0(ckr);
            ckr->chunk = nd->value.chunk;
            ckr->labels = empty_list;
            Str ctn;
            ctn = content_of_chunk(ckr);
            FREE(ckr);
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

    ChunkRef ckr = NULL;
    NEW0(ckr);
    ckr->chunk = nd->value.chunk;
    ckr->labels = empty_list;

    Str ctn;
    ctn = content_of_chunk(ckr);
    FREE(ckr);

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
    FREE(prefix);
}

int
main(int argc, char *argv[])
{
    RESERVE(do {
        new_string_hash_table(&file_to_mmap_addr);
        new_string_hash_table(&module_name_to_module);
        NEW(root_node);
        root_node->type = NODE_DIRECTORY;
        root_node->value.nodes = empty_list;
        root_node->name = atom_str("");
        if (getcwd(working_dir, PATH_MAX) == NULL) {
            perror("getcwd");
            exit(1);
        }
        ensure_directory(litar_data);
    } while (0));
    do {
        int opt, option_index;
        static struct option long_options[] = {
            {   "help",       no_argument, NULL,   0},
            {"version",       no_argument, NULL,   0},
            { "mirror", required_argument, NULL, 'm'},
            {  "print", required_argument, NULL, 'p'},
            {"execute", required_argument, NULL, 'x'},
        };

        while ((opt = getopt_long(
                    argc, argv, "m:p:x:", long_options, &option_index
                ))
               != -1) {
            switch (opt) {
            case 0:
                if (strcmp(long_options[option_index].name, "help") == 0) {
                    chosen_usage = 0;
                    to_print_help = true;
                    break;
                }
                if (strcmp(long_options[option_index].name, "version") == 0) {
                    chosen_usage = 0;
                    to_print_version_info = true;
                    break;
                }

                break;
            case 'm':
                chosen_usage = 1;
                mirror_point = optarg;
                break;
            case 'p':
                chosen_usage = 1;
                name_of_chunk_to_print = optarg;
                break;
            case 'x':
                chosen_usage = 1;
                name_of_chunk_to_execute = optarg;
                break;

            default:
                print_usage();
                exit(1);
            }
        }

        if (chosen_usage == -1) {
            print_usage();
            exit(1);
        }

        switch (chosen_usage) {
        case 0:
            break;
        case 1:
            if (argc - optind != 1) {
                print_usage();
                exit(1);
            }
            archive_name = argv[optind];

            if (access(archive_name, R_OK) == -1) {
                perror(archive_name);
                exit(1);
            }
            break;
        default:
            print_usage();
            exit(1);
        }
    } while (0);
    ;
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
                    "Version %s, built at 2026-01-26T14:34+08:00\n", version
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
                ipt->file = atom_str(archive_name);
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
                &state_parse_block_chunk_ref,
                state_parse_block,
                STATE_XOR,
                state_parse_block_chunk_ref_handler
            );
            state_register_in_func(
                state_parse_block_chunk_ref, state_parse_block_chunk_ref_in
            );
            state_register_out_func(
                state_parse_block_chunk_ref, state_parse_block_chunk_ref_out
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
                &state_parse_block_chunk_ref_name,
                state_parse_block_chunk_ref,
                STATE_XOR,
                state_parse_block_chunk_ref_name_handler
            );
            state_register_in_func(
                state_parse_block_chunk_ref_name,
                state_parse_block_chunk_ref_name_in
            );
            state_register_out_func(
                state_parse_block_chunk_ref_name,
                state_parse_block_chunk_ref_name_out
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
                &state_parse_block_contents_reference_chunk_ref,
                state_parse_block_contents_reference,
                STATE_XOR,
                state_parse_block_contents_reference_chunk_ref_handler
            );
            state_register_in_func(
                state_parse_block_contents_reference_chunk_ref,
                state_parse_block_contents_reference_chunk_ref_in
            );
            state_register_out_func(
                state_parse_block_contents_reference_chunk_ref,
                state_parse_block_contents_reference_chunk_ref_out
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

            new_state(
                &state_parse_block_contents_reference_chunk_ref_name,
                state_parse_block_contents_reference_chunk_ref,
                STATE_XOR,
                state_parse_block_contents_reference_chunk_ref_name_handler
            );
            state_register_in_func(
                state_parse_block_contents_reference_chunk_ref_name,
                state_parse_block_contents_reference_chunk_ref_name_in
            );
            state_register_out_func(
                state_parse_block_contents_reference_chunk_ref_name,
                state_parse_block_contents_reference_chunk_ref_name_out
            );

            new_state(
                &state_parse_block_contents_reference_filters_name,
                state_parse_block_contents_reference_filters,
                STATE_XOR,
                state_parse_block_contents_reference_filters_name_handler
            );
            state_register_in_func(
                state_parse_block_contents_reference_filters_name,
                state_parse_block_contents_reference_filters_name_in
            );
            state_register_out_func(
                state_parse_block_contents_reference_filters_name,
                state_parse_block_contents_reference_filters_name_out
            );

            new_state(
                &state_parse_block_filters_name,
                state_parse_block_filters,
                STATE_XOR,
                state_parse_block_filters_name_handler
            );
            state_register_in_func(
                state_parse_block_filters_name,
                state_parse_block_filters_name_in
            );
            state_register_out_func(
                state_parse_block_filters_name,
                state_parse_block_filters_name_out
            );

            new_state(
                &state_parse_escaping,
                state_parse,
                STATE_XOR,
                state_parse_escaping_handler
            );
            state_register_in_func(
                state_parse_escaping, state_parse_escaping_in
            );
            state_register_out_func(
                state_parse_escaping, state_parse_escaping_out
            );

            new_state(
                &state_parse_escaping_argument,
                state_parse_escaping,
                STATE_XOR,
                state_parse_escaping_argument_handler
            );
            state_register_in_func(
                state_parse_escaping_argument, state_parse_escaping_argument_in
            );
            state_register_out_func(
                state_parse_escaping_argument,
                state_parse_escaping_argument_out
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
                            ipt->pr + 1,
                            ipt->pc + 1,
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
            free_state(&state_parse_block_chunk_ref);
            free_state(&state_parse_block_contents);
            free_state(&state_parse_block_filters);
            free_state(&state_parse_block_chunk_ref_name);
            free_state(&state_parse_block_contents_text);
            free_state(&state_parse_block_contents_reference);
            free_state(&state_parse_block_contents_reference_chunk_ref);
            free_state(&state_parse_block_contents_reference_filters);
            free_state(&state_parse_block_contents_reference_chunk_ref_name);
            free_state(&state_parse_block_contents_reference_filters_name);
            free_state(&state_parse_block_filters_name);
            free_state(&state_parse_escaping);
            free_state(&state_parse_escaping_argument);
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
            if (mirror_point) {
                break;
            }
            if (name_of_chunk_to_print) {
                if (!chunk_of_name_exist_in_module(
                        name_of_chunk_to_print, module_of_name("")
                    )) {
                    fprintf(
                        stderr, "Unknown chunk: %s\n", name_of_chunk_to_print
                    );
                    exit(1);
                }
                Chunk chk;
                chk = chunk_in_module_of_name(
                    module_of_name(""), name_of_chunk_to_print
                );
                ChunkRef ckr = NULL;

                NEW0(ckr);
                ckr->chunk = chk;
                ckr->labels = empty_list;

                Str cc;
                cc = content_of_chunk(ckr);
                FREE(ckr);

                puts(raw_string(cc));
                free_str(&cc);
                break;
            }
            if (name_of_chunk_to_execute) {
                if (!chunk_of_name_exist_in_module(
                        name_of_chunk_to_execute, module_of_name("")
                    )) {
                    fprintf(
                        stderr, "Unknown chunk: %s\n", name_of_chunk_to_execute
                    );
                    exit(1);
                }
                Chunk chk;
                chk = chunk_in_module_of_name(
                    module_of_name(""), name_of_chunk_to_execute
                );

                ChunkRef ckr = NULL;
                NEW0(ckr);
                ckr->chunk = chk;
                ckr->labels = empty_list;

                pid_t cld;
                int st;
                cld = run_filter(ckr, STDIN_FILENO, STDOUT_FILENO);
                FREE(ckr);
                waitpid(cld, &st, 0);

                if (WIFEXITED(st)) {
                    if (WEXITSTATUS(st) != 0) {
                        fprintf(
                            stderr,
                            "Faled at execute @<%s@>, error code: %d\n",
                            name_of_chunk_to_execute,
                            WEXITSTATUS(st)
                        );
                        exit(1);
                    }
                } else {
                    fprintf(
                        stderr,
                        "Faled at execute @<%s@>, unknown reason\n",
                        name_of_chunk_to_execute
                    );
                    exit(1);
                }
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
