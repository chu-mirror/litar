/* Bootstrap of litar: the archive syntax through Filters.
   A filter is a chunk run as a script in the current directory.
   File definitions and file sets belong to later stages.
   See design/litar.md and design/stages.md. */

#define _POSIX_C_SOURCE 200809L

#include <errno.h>
#include <fcntl.h>
#include <poll.h>
#include <signal.h>
#include <stdarg.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/wait.h>
#include <unistd.h>

#if defined(__GNUC__)
#define NORETURN __attribute__((noreturn))
#else
#define NORETURN
#endif

/* A chunk reference is a label sequence, a module, and a chunk name.
   The anonymous module is the empty string. A block stored under a
   reference extends that reference only. Evaluating a reference
   concatenates, in archive order, every block of that module and chunk
   whose labels are an order-preserving subsequence of the reference.
   The empty label list is the unspecialized chunk and matches any
   specialization. A reference with no labels keeps the labels of the
   reference being expanded. A reference with no module uses the module
   that was selected where its block was written. */

typedef struct StrVec {
    const char **v;
    int n;
    int cap;
} StrVec;

typedef struct QName {
    StrVec labels;
    int has_module;
    const char *module;
    const char *chunk;
    struct QName *filters;
    int nfilters;
    int capfilters;
} QName;

/* A filter is a chunk reference. Labels and the module are optional.
   The strings live in the archive arena. */
typedef struct Filt {
    const char **labels;
    int nlabels;
    int has_module;
    const char *module;
    const char *chunk;
} Filt;

typedef struct Block {
    const char **labels;
    int nlabels;
    const char *module;
    const char *chunk;
    const char *content;
    size_t content_len;
    Filt *filters;
    int nfilters;
    int def_line;
    const char *def_file;
    const char *def_module;
} Block;

typedef struct Buf {
    char *data;
    size_t n;
    size_t cap;
} Buf;

typedef struct Group {
    const char *name;
    const char **parts;
    int nparts;
    struct Group *next;
} Group;

typedef struct Flag {
    const char *name;
    struct Flag *next;
} Flag;

typedef struct ArenaChunk {
    struct ArenaChunk *next;
    size_t used;
    size_t cap;
    char data[];
} ArenaChunk;

typedef struct Arena {
    ArenaChunk *head;
} Arena;

typedef struct Archive {
    Arena arena;
    char *src;
    size_t src_n;
    Block *blocks;
    int nblocks;
    int capblocks;
    Group *groups;
    Flag *flags;
    const char *current_module;
    const char *anonymous_module;
    int inc_depth;
} Archive;

typedef struct Parser {
    const char *buf;
    size_t n;
    size_t i;
    Archive *arc;
    int report_lines;
    const char *filename;
    const char *where;
    int def_line;
    const char *def_file;
} Parser;

typedef struct Visit {
    char **keys;
    int n;
    int cap;
} Visit;

enum {
    Q_BLOCK = 1,
    Q_REF,
    Q_EXPR
};

static void fail(const char *fmt, ...) NORETURN;

static void fail(const char *fmt, ...) {
    va_list ap;
    fprintf(stderr, "Error: ");
    va_start(ap, fmt);
    vfprintf(stderr, fmt, ap);
    va_end(ap);
    fputc('\n', stderr);
    exit(1);
}

static int line_at(const Parser *p, size_t pos) {
    int line = 1;
    if (pos > p->n) {
        pos = p->n;
    }
    for (size_t i = 0; i < pos; i++) {
        if (p->buf[i] == '\n') {
            line++;
        }
    }
    return line;
}

static void fail_at(const Parser *p, const char *fmt, ...) NORETURN;

static void fail_at(const Parser *p, const char *fmt, ...) {
    va_list ap;
    fprintf(stderr, "Error: ");
    va_start(ap, fmt);
    vfprintf(stderr, fmt, ap);
    va_end(ap);
    if (p && p->report_lines) {
        fprintf(stderr, " at line %d", line_at(p, p->i));
        if (p->filename) {
            fprintf(stderr, " in '%s'", p->filename);
        }
    } else if (p && p->where) {
        fprintf(stderr, " in chunk '%s'", p->where);
        if (p->def_line) {
            fprintf(stderr, " (defined at line %d", p->def_line);
            if (p->def_file) {
                fprintf(stderr, " in '%s'", p->def_file);
            }
            fputc(')', stderr);
        }
    } else if (p) {
        fprintf(stderr, " in expression");
    }
    fputc('\n', stderr);
    exit(1);
}

static void *xmalloc(size_t n) {
    void *p = malloc(n ? n : 1);
    if (!p) {
        fail("out of memory");
    }
    return p;
}

static char *xstrdup(const char *s) {
    size_t n = strlen(s);
    char *d = xmalloc(n + 1);
    memcpy(d, s, n + 1);
    return d;
}

static void *arena_alloc(Arena *a, size_t n) {
    if (n == 0) {
        n = 1;
    }
    n = (n + 7u) & ~(size_t)7u;
    if (!a->head || a->head->used + n > a->head->cap) {
        size_t cap = 8192;
        if (n > cap) {
            cap = n;
        }
        ArenaChunk *c = malloc(sizeof(ArenaChunk) + cap);
        if (!c) {
            fail("out of memory");
        }
        c->next = a->head;
        c->used = 0;
        c->cap = cap;
        a->head = c;
    }
    void *p = a->head->data + a->head->used;
    a->head->used += n;
    memset(p, 0, n);
    return p;
}

static char *arena_strndup(Arena *a, const char *s, size_t n) {
    char *d = arena_alloc(a, n + 1);
    if (n) {
        memcpy(d, s, n);
    }
    d[n] = '\0';
    return d;
}

static void arena_free(Arena *a) {
    ArenaChunk *c = a->head;
    while (c) {
        ArenaChunk *next = c->next;
        free(c);
        c = next;
    }
    a->head = NULL;
}

static void sv_push(StrVec *sv, const char *s) {
    if (sv->n == sv->cap) {
        int ncap = sv->cap ? sv->cap * 2 : 8;
        const char **nv = xmalloc((size_t)ncap * sizeof(char *));
        if (sv->n) {
            memcpy(nv, sv->v, (size_t)sv->n * sizeof(char *));
        }
        free(sv->v);
        sv->v = nv;
        sv->cap = ncap;
    }
    sv->v[sv->n++] = s;
}

static int is_ascii_alpha(unsigned char c) {
    return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z');
}

/* '@' followed by an ASCII non-letter is a control. '@' followed by a
   letter, or by a non-ASCII byte, is ordinary text. 0 means the buffer
   ends on '@', which closes an expression. -1 means this is not a control.
   '@@' is the text '@', not a control: kind '@' is that escape. */

static int control_kind(const Parser *p) {
    unsigned char c;
    if (p->i >= p->n || p->buf[p->i] != '@') {
        return -1;
    }
    if (p->i + 1 >= p->n) {
        return 0;
    }
    c = (unsigned char)p->buf[p->i + 1];
    if (c >= 128 || is_ascii_alpha(c)) {
        return -1;
    }
    return (int)c;
}

static int is_end_kind(int k) {
    return k == 0 || k == ' ' || k == '\t' || k == '\n' || k == '\r';
}

static int is_trim_space(unsigned char c) {
    return c == ' ' || c == '\t' || c == '\n' || c == '\r';
}

/* Stop at the next control, but pass over @@. Both bytes of the escape
   stay in the span so the name can be decoded to a single '@'. */

static void scan_to_control(Parser *p) {
    while (p->i < p->n) {
        int k = control_kind(p);
        if (k == '@') {
            p->i += 2;
            continue;
        }
        if (k >= 0) {
            return;
        }
        p->i++;
    }
}

static char *take_name(Parser *p, size_t start, size_t end) {
    size_t n = 0;
    size_t w = 0;
    char *d;
    while (start < end && is_trim_space((unsigned char)p->buf[start])) {
        start++;
    }
    while (end > start && is_trim_space((unsigned char)p->buf[end - 1])) {
        end--;
    }
    for (size_t i = start; i < end; i++) {
        if (p->buf[i] == '@' && i + 1 < end && p->buf[i + 1] == '@') {
            i++;
        }
        n++;
    }
    d = arena_alloc(&p->arc->arena, n + 1);
    for (size_t i = start; i < end; i++) {
        if (p->buf[i] == '@' && i + 1 < end && p->buf[i + 1] == '@') {
            d[w++] = '@';
            i++;
            continue;
        }
        d[w++] = p->buf[i];
    }
    d[w] = '\0';
    return d;
}

static char *read_name_until_control(Parser *p) {
    size_t start = p->i;
    scan_to_control(p);
    if (p->i >= p->n) {
        fail_at(p, "unterminated expression");
    }
    return take_name(p, start, p->i);
}

static void consume_end(Parser *p) {
    int k = control_kind(p);
    if (k < 0 || !is_end_kind(k)) {
        fail_at(p, "expected '@' to end the expression");
    }
    if (k == 0) {
        p->i = p->n;
        return;
    }
    p->i += 2;
}

static void qname_add_filter(QName *qn, const QName *filt) {
    if (qn->nfilters == qn->capfilters) {
        int ncap = qn->capfilters ? qn->capfilters * 2 : 4;
        QName *nf = xmalloc((size_t)ncap * sizeof(QName));
        if (qn->nfilters) {
            memcpy(nf, qn->filters, (size_t)qn->nfilters * sizeof(QName));
        }
        free(qn->filters);
        qn->filters = nf;
        qn->capfilters = ncap;
    }
    qn->filters[qn->nfilters++] = *filt;
}

static void qname_free(QName *qn) {
    free(qn->labels.v);
    qn->labels.v = NULL;
    for (int i = 0; i < qn->nfilters; i++) {
        free(qn->filters[i].labels.v);
    }
    free(qn->filters);
    qn->filters = NULL;
    qn->nfilters = 0;
    qn->capfilters = 0;
}

/* A filter name is a qualified name. It ends at the next filter,
   at the end of a reference, or at the end of the expression.
   It does not itself take filters: `@|a@|b` is a pipeline. */

static void parse_filter_name(Parser *p, QName *filt, int expr) {
    memset(filt, 0, sizeof(*filt));
    for (;;) {
        size_t start = p->i;
        int k;
        char *name;
        scan_to_control(p);
        name = take_name(p, start, p->i);
        if (p->i >= p->n) {
            if (!expr) {
                fail_at(p, "unterminated name");
            }
            if (!name[0]) {
                fail_at(p, "empty filter name");
            }
            filt->chunk = name;
            return;
        }
        k = control_kind(p);
        if (k == ':') {
            if (filt->has_module) {
                fail_at(p, "label after module name");
            }
            if (!name[0]) {
                fail_at(p, "empty label");
            }
            sv_push(&filt->labels, name);
            p->i += 2;
            continue;
        }
        if (k == '/') {
            if (filt->has_module) {
                fail_at(p, "extra '@/' in name");
            }
            if (!name[0]) {
                fail_at(p, "empty module name");
            }
            filt->has_module = 1;
            filt->module = name;
            p->i += 2;
            continue;
        }
        if (k == '|' || k == '>' || is_end_kind(k)) {
            if (!name[0]) {
                fail_at(p, "empty filter name");
            }
            filt->chunk = name;
            return;
        }
        fail_at(p, "unexpected control '@%c' in name", k);
    }
}

static void parse_filter_list(Parser *p, QName *qn, int expr) {
    for (;;) {
        QName filt;
        int k;
        parse_filter_name(p, &filt, expr);
        qname_add_filter(qn, &filt);
        if (p->i >= p->n) {
            return;
        }
        k = control_kind(p);
        if (k == '|') {
            p->i += 2;
            continue;
        }
        return;
    }
}

static void parse_qname(Parser *p, QName *qn, int mode) {
    memset(qn, 0, sizeof(*qn));
    for (;;) {
        size_t start = p->i;
        int k;
        char *name;
        scan_to_control(p);
        name = take_name(p, start, p->i);
        if (p->i >= p->n) {
            if (mode != Q_EXPR) {
                fail_at(p, "unterminated name");
            }
            if (!name[0]) {
                fail_at(p, "empty chunk name");
            }
            qn->chunk = name;
            return;
        }
        k = control_kind(p);
        if (k == '|') {
            /* On a block header the filter list belongs after the body.
               On a reference or a print expression it belongs here. */
            if (mode == Q_BLOCK) {
                fail_at(p, "unexpected control '@|' in name");
            }
            if (!name[0]) {
                fail_at(p, "empty chunk name");
            }
            qn->chunk = name;
            p->i += 2;
            parse_filter_list(p, qn, mode == Q_EXPR);
            return;
        }
        if (k == ':') {
            if (qn->has_module) {
                fail_at(p, "label after module name");
            }
            if (!name[0]) {
                fail_at(p, "empty label");
            }
            sv_push(&qn->labels, name);
            p->i += 2;
            continue;
        }
        if (k == '/') {
            if (qn->has_module) {
                fail_at(p, "extra '@/' in name");
            }
            if (!name[0]) {
                fail_at(p, "empty module name");
            }
            qn->has_module = 1;
            qn->module = name;
            p->i += 2;
            continue;
        }
        if (mode == Q_BLOCK && k == '=') {
            if (!name[0]) {
                fail_at(p, "empty chunk name");
            }
            qn->chunk = name;
            return;
        }
        if (mode == Q_REF && k == '>') {
            if (!name[0]) {
                fail_at(p, "empty chunk name");
            }
            qn->chunk = name;
            return;
        }
        if (k == 0) {
            fail_at(p, "unterminated name");
        }
        fail_at(p, "unexpected control '@%c' in name", k);
    }
}

static void skip_to_control(Parser *p) {
    scan_to_control(p);
}

static void add_block(Archive *a, Block blk) {
    if (a->nblocks == a->capblocks) {
        int ncap = a->capblocks ? a->capblocks * 2 : 16;
        Block *nbl = xmalloc((size_t)ncap * sizeof(Block));
        if (a->nblocks) {
            memcpy(nbl, a->blocks, (size_t)a->nblocks * sizeof(Block));
        }
        free(a->blocks);
        a->blocks = nbl;
        a->capblocks = ncap;
    }
    a->blocks[a->nblocks++] = blk;
}

static void copy_filters(Arena *a, QName *filters, int n, Filt **out, int *nout) {
    Filt *fs;
    if (n == 0) {
        *out = NULL;
        *nout = 0;
        return;
    }
    fs = arena_alloc(a, (size_t)n * sizeof(Filt));
    for (int i = 0; i < n; i++) {
        QName *f = &filters[i];
        fs[i].nlabels = f->labels.n;
        fs[i].labels = NULL;
        if (f->labels.n) {
            const char **labs = arena_alloc(a, (size_t)f->labels.n * sizeof(char *));
            for (int j = 0; j < f->labels.n; j++) {
                labs[j] = f->labels.v[j];
            }
            fs[i].labels = labs;
        }
        fs[i].has_module = f->has_module;
        fs[i].module = f->module;
        fs[i].chunk = f->chunk;
    }
    *out = fs;
    *nout = n;
}

static void parse_block(Parser *p, int execute) {
    QName qn;
    Block blk;
    size_t body;
    size_t end;
    int depth = 0;
    int line = line_at(p, p->i);
    p->i += 2;
    parse_qname(p, &qn, Q_BLOCK);
    if (control_kind(p) != '=') {
        fail_at(p, "expected '@='");
    }
    p->i += 2;
    body = p->i;
    /* `@|` ends the body only outside a reference. `@|` between `@<`
       and `@>` belongs to that reference and stays in the body.
       `@` followed by whitespace still ends the block at any depth,
       so an unclosed reference does not swallow the block's closer. */
    while (p->i < p->n) {
        int k = control_kind(p);
        if (k < 0) {
            p->i++;
            continue;
        }
        if (is_end_kind(k)) {
            break;
        }
        if (k == '|' && depth == 0) {
            break;
        }
        if (k == '<') {
            depth++;
        } else if (k == '>' && depth > 0) {
            depth--;
        }
        p->i += 2;
    }
    if (p->i >= p->n) {
        fail_at(p, "unterminated block");
    }
    /* The newline after '@=' is layout, not content.
       The newline before the closing '@' or '@|' is content.
       @<msg@=\nHello\n@ therefore stores Hello\n. */
    end = p->i;
    if (body < end && p->buf[body] == '\r') {
        body++;
    }
    if (body < end && p->buf[body] == '\n') {
        body++;
    }
    if (control_kind(p) == '|') {
        p->i += 2;
        parse_filter_list(p, &qn, 0);
    }
    if (p->i >= p->n || !is_end_kind(control_kind(p))) {
        fail_at(p, "expected '@' to end the expression");
    }
    consume_end(p);
    if (!execute) {
        qname_free(&qn);
        return;
    }
    memset(&blk, 0, sizeof(blk));
    blk.nlabels = qn.labels.n;
    if (qn.labels.n) {
        const char **labs = arena_alloc(&p->arc->arena,
                                         (size_t)qn.labels.n * sizeof(char *));
        for (int i = 0; i < qn.labels.n; i++) {
            labs[i] = qn.labels.v[i];
        }
        blk.labels = labs;
    }
    blk.module = qn.has_module ? qn.module : p->arc->current_module;
    blk.chunk = qn.chunk;
    blk.content_len = end - body;
    blk.content = arena_strndup(&p->arc->arena, p->buf + body, blk.content_len);
    copy_filters(&p->arc->arena, qn.filters, qn.nfilters, &blk.filters, &blk.nfilters);
    blk.def_line = line;
    blk.def_file = p->filename;
    blk.def_module = p->arc->current_module;
    qname_free(&qn);
    add_block(p->arc, blk);
}

static void parse_module_sel(Parser *p, int execute) {
    char *name;
    p->i += 2;
    name = read_name_until_control(p);
    if (!is_end_kind(control_kind(p))) {
        fail_at(p, "expected '@' to end the expression");
    }
    consume_end(p);
    if (execute) {
        p->arc->current_module = name;
    }
}

static void parse_flag(Parser *p, int execute) {
    char *name;
    Flag *f;
    p->i += 2;
    name = read_name_until_control(p);
    if (!name[0]) {
        fail_at(p, "empty flag name");
    }
    if (!is_end_kind(control_kind(p))) {
        fail_at(p, "expected '@' to end the expression");
    }
    consume_end(p);
    if (!execute) {
        return;
    }
    for (f = p->arc->flags; f; f = f->next) {
        if (strcmp(f->name, name) == 0) {
            return;
        }
    }
    f = arena_alloc(&p->arc->arena, sizeof(Flag));
    f->name = name;
    f->next = p->arc->flags;
    p->arc->flags = f;
}

static int flag_is_set(Archive *a, const char *name) {
    for (Flag *f = a->flags; f; f = f->next) {
        if (strcmp(f->name, name) == 0) {
            return 1;
        }
    }
    return 0;
}

static Group *find_group(Archive *a, const char *name) {
    for (Group *g = a->groups; g; g = g->next) {
        if (strcmp(g->name, name) == 0) {
            return g;
        }
    }
    return NULL;
}

static void parse_group(Parser *p, int execute) {
    char *name;
    StrVec parts = {0};
    Group *g;
    p->i += 2;
    name = read_name_until_control(p);
    if (!name[0]) {
        fail_at(p, "empty label group name");
    }
    if (control_kind(p) != '=') {
        fail_at(p, "expected '@=' in label group");
    }
    p->i += 2;
    for (;;) {
        char *part = read_name_until_control(p);
        int k;
        if (!part[0]) {
            fail_at(p, "empty label");
        }
        sv_push(&parts, part);
        k = control_kind(p);
        if (k == ':') {
            p->i += 2;
            continue;
        }
        if (is_end_kind(k)) {
            break;
        }
        fail_at(p, "unexpected control in label group");
    }
    consume_end(p);
    if (!execute) {
        free(parts.v);
        return;
    }
    if (find_group(p->arc, name)) {
        fail_at(p, "label group '%s' is already defined", name);
    }
    g = arena_alloc(&p->arc->arena, sizeof(Group));
    g->name = name;
    g->nparts = parts.n;
    g->parts = arena_alloc(&p->arc->arena, (size_t)parts.n * sizeof(char *));
    for (int i = 0; i < parts.n; i++) {
        g->parts[i] = parts.v[i];
    }
    g->next = p->arc->groups;
    p->arc->groups = g;
    free(parts.v);
}

static void parse_items(Parser *p, int execute, int in_branch);

/* Arms are exclusive: the first @? whose flag is set is kept, and @|
   is kept when none of those flags are set. An arm ends at the next
   @?, @|, or closing @, so a branch does not contain another branch. */

static void parse_branch(Parser *p, int execute) {
    int taken = 0;
    int saw_else = 0;
    for (;;) {
        int k;
        skip_to_control(p);
        if (p->i >= p->n) {
            fail_at(p, "unterminated branch");
        }
        k = control_kind(p);
        if (k == '?') {
            char *cond;
            int arm;
            if (saw_else) {
                fail_at(p, "condition after else");
            }
            p->i += 2;
            cond = read_name_until_control(p);
            if (!cond[0]) {
                fail_at(p, "empty condition");
            }
            if (control_kind(p) != '|') {
                fail_at(p, "expected '@|' after condition");
            }
            p->i += 2;
            arm = execute && !taken && flag_is_set(p->arc, cond);
            if (arm) {
                taken = 1;
            }
            parse_items(p, arm, 1);
        } else if (k == '|') {
            int arm;
            if (saw_else) {
                fail_at(p, "duplicate else");
            }
            saw_else = 1;
            p->i += 2;
            arm = execute && !taken;
            if (arm) {
                taken = 1;
            }
            parse_items(p, arm, 1);
        } else if (is_end_kind(k)) {
            consume_end(p);
            return;
        } else {
            fail_at(p, "unexpected control in branch");
        }
    }
}

static void parse_include(Parser *p, int execute);

static void parse_one(Parser *p, int execute) {
    int k = control_kind(p);
    if (k == '<') {
        parse_block(p, execute);
    } else if (k == '-') {
        parse_module_sel(p, execute);
    } else if (k == '!') {
        parse_flag(p, execute);
    } else if (k == '?') {
        parse_branch(p, execute);
    } else if (k == ':') {
        parse_group(p, execute);
    } else if (k == '.') {
        parse_include(p, execute);
    } else if (k == '[' || k == '(') {
        fail_at(p, "file definitions are not implemented");
    } else if (k == '`' || k == '+' || k == ',') {
        fail_at(p, "file sets are not implemented");
    } else if (k == '|') {
        fail_at(p, "unexpected '@|'");
    } else if (is_end_kind(k)) {
        fail_at(p, "unexpected '@'");
    } else if (k > 0) {
        fail_at(p, "unknown expression '@%c'", k);
    } else {
        fail_at(p, "unterminated expression");
    }
}

static void parse_items(Parser *p, int execute, int in_branch) {
    for (;;) {
        int k;
        size_t before;
        skip_to_control(p);
        if (p->i >= p->n) {
            if (in_branch) {
                fail_at(p, "unterminated branch");
            }
            return;
        }
        k = control_kind(p);
        if (in_branch && (k == '?' || k == '|' || is_end_kind(k))) {
            return;
        }
        before = p->i;
        parse_one(p, execute);
        if (p->i <= before) {
            fail_at(p, "internal parse stall");
        }
    }
}

static void expand_name(Archive *a, const char *name, StrVec *out,
                        const char **stack, int sp) {
    Group *g = find_group(a, name);
    const char *nst[128];
    if (!g) {
        sv_push(out, name);
        return;
    }
    for (int i = 0; i < sp; i++) {
        if (strcmp(stack[i], name) == 0) {
            fail("circular label group '%s'", name);
        }
    }
    if (sp >= 128) {
        fail("label group '%s' is nested too deeply", name);
    }
    if (sp) {
        memcpy(nst, stack, (size_t)sp * sizeof(char *));
    }
    nst[sp] = name;
    for (int i = 0; i < g->nparts; i++) {
        expand_name(a, g->parts[i], out, nst, sp + 1);
    }
}

static void expand_label_list(Archive *a, const char **in, int n, StrVec *out) {
    for (int i = 0; i < n; i++) {
        expand_name(a, in[i], out, NULL, 0);
    }
}

/* Block labels match when each one occurs, in order, in the target.
   Dropping any labels from the target still matches, so both a prefix
   and a suffix are predecessors. The empty list always matches. */

static int is_subsequence(const char **block, int bn, const char **target, int tn) {
    int j = 0;
    for (int i = 0; i < bn; i++) {
        while (j < tn && strcmp(block[i], target[j]) != 0) {
            j++;
        }
        if (j >= tn) {
            return 0;
        }
        j++;
    }
    return 1;
}

static char *canonical(const char **labels, int n, const char *module,
                       const char *chunk) {
    size_t need = strlen(chunk) + 1;
    int has_mod = module && module[0];
    char *s;
    char *w;
    for (int i = 0; i < n; i++) {
        need += strlen(labels[i]) + 2;
    }
    if (has_mod) {
        need += strlen(module) + 2;
    }
    s = xmalloc(need);
    w = s;
    for (int i = 0; i < n; i++) {
        size_t L = strlen(labels[i]);
        memcpy(w, labels[i], L);
        w += L;
        memcpy(w, "@:", 2);
        w += 2;
    }
    if (has_mod) {
        size_t L = strlen(module);
        memcpy(w, module, L);
        w += L;
        memcpy(w, "@/", 2);
        w += 2;
    }
    memcpy(w, chunk, strlen(chunk) + 1);
    return s;
}

static int visit_has(Visit *v, const char *key) {
    for (int i = 0; i < v->n; i++) {
        if (strcmp(v->keys[i], key) == 0) {
            return 1;
        }
    }
    return 0;
}

static void visit_push(Visit *v, char *key) {
    if (v->n == v->cap) {
        int ncap = v->cap ? v->cap * 2 : 8;
        char **nk = xmalloc((size_t)ncap * sizeof(char *));
        if (v->n) {
            memcpy(nk, v->keys, (size_t)v->n * sizeof(char *));
        }
        free(v->keys);
        v->keys = nk;
        v->cap = ncap;
    }
    v->keys[v->n++] = key;
}

static void visit_pop(Visit *v) {
    free(v->keys[--v->n]);
}

static void buf_append(Buf *b, const char *s, size_t n) {
    size_t cap;
    char *d;
    if (n == 0) {
        return;
    }
    if (b->n + n < b->n) {
        fail("out of memory");
    }
    if (b->n + n <= b->cap) {
        memcpy(b->data + b->n, s, n);
        b->n += n;
        return;
    }
    cap = b->cap ? b->cap : 64;
    while (cap < b->n + n) {
        if (cap > (SIZE_MAX / 2)) {
            fail("out of memory");
        }
        cap *= 2;
    }
    d = realloc(b->data, cap);
    if (!d) {
        fail("out of memory");
    }
    b->data = d;
    b->cap = cap;
    memcpy(b->data + b->n, s, n);
    b->n += n;
}

static int write_all(int fd, const char *buf, size_t n) {
    size_t off = 0;
    while (off < n) {
        ssize_t w = write(fd, buf + off, n - off);
        if (w < 0) {
            if (errno == EINTR) {
                continue;
            }
            return -1;
        }
        if (w == 0) {
            errno = EIO;
            return -1;
        }
        off += (size_t)w;
    }
    return 0;
}

/* `#! interpreter` or `#! interpreter arg`. One optional word, as the
   kernel's script loader does. Spaces after `#!` are skipped. */

static int copy_word(const char *s, size_t n, char *dst, size_t cap) {
    if (n == 0 || n >= cap) {
        return 0;
    }
    if (s[n - 1] == '\r') {
        n--;
        if (n == 0 || n >= cap) {
            return 0;
        }
    }
    memcpy(dst, s, n);
    dst[n] = '\0';
    return 1;
}

static int parse_shebang(const char *script, size_t n, char *interp, size_t interp_cap,
                         char *arg, size_t arg_cap, int *has_arg) {
    size_t i = 2;
    size_t start;
    *has_arg = 0;
    if (n < 2 || script[0] != '#' || script[1] != '!') {
        return 0;
    }
    while (i < n && (script[i] == ' ' || script[i] == '\t')) {
        i++;
    }
    start = i;
    while (i < n && script[i] != ' ' && script[i] != '\t' && script[i] != '\n' &&
           script[i] != '\r') {
        i++;
    }
    if (!copy_word(script + start, i - start, interp, interp_cap)) {
        return 0;
    }
    if (i < n && script[i] == '\r') {
        i++;
    }
    if (i >= n || script[i] == '\n') {
        return 1;
    }
    while (i < n && (script[i] == ' ' || script[i] == '\t')) {
        i++;
    }
    if (i >= n || script[i] == '\n' || script[i] == '\r') {
        return 1;
    }
    start = i;
    while (i < n && script[i] != ' ' && script[i] != '\t' && script[i] != '\n' &&
           script[i] != '\r') {
        i++;
    }
    if (!copy_word(script + start, i - start, arg, arg_cap)) {
        return 0;
    }
    *has_arg = 1;
    return 1;
}

enum {
    RUN_OK = 0,
    RUN_STATUS = 1,
    RUN_SIGNAL = 2,
    RUN_EXEC = 3,
    RUN_IO = 4,
    RUN_SHEBANG = 5
};

static int pump_filter(int in_fd, int out_fd, const char *input, size_t input_n, Buf *out) {
    size_t off = 0;
    struct sigaction ign;
    struct sigaction old;
    int restore = 0;
    int rc = 0;
    memset(&ign, 0, sizeof(ign));
    ign.sa_handler = SIG_IGN;
    sigemptyset(&ign.sa_mask);
    if (sigaction(SIGPIPE, &ign, &old) == 0) {
        restore = 1;
    }
    if (input_n == 0 && in_fd >= 0) {
        close(in_fd);
        in_fd = -1;
    }
    while (in_fd >= 0 || out_fd >= 0) {
        struct pollfd pf[2];
        int np = 0;
        int in_ix = -1;
        int out_ix = -1;
        int pr;
        if (in_fd >= 0) {
            in_ix = np;
            pf[np].fd = in_fd;
            pf[np].events = POLLOUT;
            pf[np].revents = 0;
            np++;
        }
        if (out_fd >= 0) {
            out_ix = np;
            pf[np].fd = out_fd;
            pf[np].events = POLLIN;
            pf[np].revents = 0;
            np++;
        }
        pr = poll(pf, (nfds_t)np, -1);
        if (pr < 0) {
            if (errno == EINTR) {
                continue;
            }
            rc = -1;
            break;
        }
        if (in_ix >= 0 && (pf[in_ix].revents & (POLLOUT | POLLERR | POLLHUP))) {
            if ((pf[in_ix].revents & POLLOUT) && off < input_n) {
                ssize_t w = write(in_fd, input + off, input_n - off);
                if (w < 0) {
                    if (errno == EPIPE) {
                        close(in_fd);
                        in_fd = -1;
                    } else if (errno != EINTR) {
                        rc = -1;
                        break;
                    }
                } else {
                    off += (size_t)w;
                    if (off == input_n) {
                        close(in_fd);
                        in_fd = -1;
                    }
                }
            } else if (pf[in_ix].revents & (POLLERR | POLLHUP)) {
                close(in_fd);
                in_fd = -1;
            }
        }
        if (out_ix >= 0 && (pf[out_ix].revents & (POLLIN | POLLHUP | POLLERR))) {
            char tmp[4096];
            ssize_t r = read(out_fd, tmp, sizeof tmp);
            if (r < 0) {
                if (errno != EINTR) {
                    rc = -1;
                    break;
                }
            } else if (r == 0) {
                close(out_fd);
                out_fd = -1;
            } else {
                buf_append(out, tmp, (size_t)r);
            }
        }
    }
    if (in_fd >= 0) {
        close(in_fd);
    }
    if (out_fd >= 0) {
        close(out_fd);
    }
    if (restore) {
        sigaction(SIGPIPE, &old, NULL);
    }
    return rc;
}

/* Run the script in the current directory. The interpreter named by
   the shebang reads the script, so the script does not have to be
   executable and a noexec temporary directory still works. The
   script's standard error is this process's standard error. */

static int run_script(const char *script, size_t script_n, const char *input, size_t input_n,
                      Buf *out, int *detail, int *err_no) {
    char interp[4096];
    char arg[4096];
    int has_arg = 0;
    const char *tmpdir;
    char *path = NULL;
    int fd = -1;
    int in_pipe[2] = {-1, -1};
    int out_pipe[2] = {-1, -1};
    int err_pipe[2] = {-1, -1};
    pid_t pid = -1;
    int status = 0;
    int rc = RUN_IO;
    size_t nd;
    if (!parse_shebang(script, script_n, interp, sizeof interp, arg, sizeof arg, &has_arg)) {
        return RUN_SHEBANG;
    }
    tmpdir = getenv("TMPDIR");
    if (!tmpdir || !tmpdir[0]) {
        tmpdir = "/tmp";
    }
    nd = strlen(tmpdir);
    path = xmalloc(nd + sizeof("/litar-XXXXXX"));
    memcpy(path, tmpdir, nd);
    memcpy(path + nd, "/litar-XXXXXX", sizeof("/litar-XXXXXX"));
    fd = mkstemp(path);
    if (fd < 0) {
        *err_no = errno;
        free(path);
        return RUN_IO;
    }
    if (write_all(fd, script, script_n) < 0) {
        *err_no = errno;
        close(fd);
        unlink(path);
        free(path);
        return RUN_IO;
    }
    if (close(fd) < 0) {
        *err_no = errno;
        unlink(path);
        free(path);
        return RUN_IO;
    }
    if (pipe(in_pipe) < 0) {
        *err_no = errno;
        in_pipe[0] = in_pipe[1] = -1;
        goto cleanup;
    }
    if (pipe(out_pipe) < 0) {
        *err_no = errno;
        out_pipe[0] = out_pipe[1] = -1;
        goto cleanup;
    }
    if (pipe(err_pipe) < 0) {
        *err_no = errno;
        err_pipe[0] = err_pipe[1] = -1;
        goto cleanup;
    }
    if (fcntl(err_pipe[1], F_SETFD, FD_CLOEXEC) < 0) {
        *err_no = errno;
        goto cleanup;
    }
    pid = fork();
    if (pid < 0) {
        *err_no = errno;
        goto cleanup;
    }
    if (pid == 0) {
        char *av[4];
        int ac = 0;
        int e;
        if (dup2(in_pipe[0], STDIN_FILENO) < 0 || dup2(out_pipe[1], STDOUT_FILENO) < 0) {
            e = errno;
            if (write(err_pipe[1], &e, sizeof e) < 0) {
            }
            _exit(127);
        }
        close(in_pipe[0]);
        close(in_pipe[1]);
        close(out_pipe[0]);
        close(out_pipe[1]);
        close(err_pipe[0]);
        av[ac++] = interp;
        if (has_arg) {
            av[ac++] = arg;
        }
        av[ac++] = path;
        av[ac] = NULL;
        execvp(av[0], av);
        e = errno;
        if (write(err_pipe[1], &e, sizeof e) < 0) {
        }
        _exit(127);
    }
    close(in_pipe[0]);
    in_pipe[0] = -1;
    close(out_pipe[1]);
    out_pipe[1] = -1;
    close(err_pipe[1]);
    err_pipe[1] = -1;
    {
        int e = 0;
        size_t got = 0;
        while (got < sizeof e) {
            ssize_t r = read(err_pipe[0], (char *)&e + got, sizeof e - got);
            if (r < 0) {
                if (errno == EINTR) {
                    continue;
                }
                *err_no = errno;
                goto cleanup;
            }
            if (r == 0) {
                break;
            }
            got += (size_t)r;
        }
        close(err_pipe[0]);
        err_pipe[0] = -1;
        if (got == sizeof e) {
            *err_no = e;
            rc = RUN_EXEC;
            goto cleanup;
        }
    }
    /* Keep the script file until the interpreter has exited. The shell
       opens the path after exec returns, so unlinking here would race. */
    if (pump_filter(in_pipe[1], out_pipe[0], input, input_n, out) < 0) {
        *err_no = errno;
        in_pipe[1] = -1;
        out_pipe[0] = -1;
        rc = RUN_IO;
        goto cleanup;
    }
    in_pipe[1] = -1;
    out_pipe[0] = -1;
    while (waitpid(pid, &status, 0) < 0) {
        if (errno != EINTR) {
            *err_no = errno;
            rc = RUN_IO;
            goto cleanup;
        }
    }
    pid = -1;
    unlink(path);
    free(path);
    path = NULL;
    if (WIFEXITED(status)) {
        if (WEXITSTATUS(status) == 0) {
            return RUN_OK;
        }
        *detail = WEXITSTATUS(status);
        return RUN_STATUS;
    }
    if (WIFSIGNALED(status)) {
        *detail = WTERMSIG(status);
        return RUN_SIGNAL;
    }
    *err_no = EIO;
    return RUN_IO;

cleanup:
    if (pid > 0) {
        kill(pid, SIGKILL);
        while (waitpid(pid, NULL, 0) < 0 && errno == EINTR) {
        }
    }
    if (in_pipe[0] >= 0) {
        close(in_pipe[0]);
    }
    if (in_pipe[1] >= 0) {
        close(in_pipe[1]);
    }
    if (out_pipe[0] >= 0) {
        close(out_pipe[0]);
    }
    if (out_pipe[1] >= 0) {
        close(out_pipe[1]);
    }
    if (err_pipe[0] >= 0) {
        close(err_pipe[0]);
    }
    if (err_pipe[1] >= 0) {
        close(err_pipe[1]);
    }
    if (path) {
        unlink(path);
        free(path);
    }
    return rc;
}

static void expand_ref(Archive *a, Visit *v, const char **labels, int nlabels,
                       const char *module, const char *chunk, Buf *out);

static int chunk_defined(Archive *a, const char **labels, int nlabels, const char *module,
                         const char *chunk);

static void run_filter(Archive *a, Visit *v, Buf *data, const char **flabels, int fn,
                       int has_module, const char *fmodule, const char *fchunk,
                       const char **cur_labels, int cur_n, const char *def_module,
                       const Parser *loc) {
    StrVec resolved = {0};
    const char **use_l;
    int use_n;
    const char *use_m;
    char *key;
    Buf program = {0};
    Buf result = {0};
    int detail = 0;
    int err_no = 0;
    int rc;
    if (!def_module) {
        def_module = "";
    }
    if (fn > 0) {
        expand_label_list(a, flabels, fn, &resolved);
        use_l = resolved.v;
        use_n = resolved.n;
    } else {
        use_l = cur_labels;
        use_n = cur_n;
    }
    use_m = has_module ? fmodule : def_module;
    if (!use_m) {
        use_m = "";
    }
    key = canonical(use_l, use_n, use_m, fchunk);
    if (!chunk_defined(a, use_l, use_n, use_m, fchunk)) {
        fail_at(loc, "filter '%s' is not defined", key);
    }
    expand_ref(a, v, use_l, use_n, use_m, fchunk, &program);
    free(resolved.v);
    if (program.n < 2 || !program.data || program.data[0] != '#' || program.data[1] != '!') {
        fail_at(loc, "filter '%s' does not start with a shebang", key);
    }
    rc = run_script(program.data, program.n, data->data ? data->data : "", data->n, &result,
                    &detail, &err_no);
    free(program.data);
    if (rc == RUN_SHEBANG) {
        fail_at(loc, "filter '%s' does not start with a shebang", key);
    }
    if (rc == RUN_STATUS) {
        fail_at(loc, "filter '%s' exited with status %d", key, detail);
    }
    if (rc == RUN_SIGNAL) {
        fail_at(loc, "filter '%s' exited with signal %d", key, detail);
    }
    if (rc != RUN_OK) {
        fail_at(loc, "could not run filter '%s': %s", key, strerror(err_no ? err_no : EIO));
    }
    free(data->data);
    *data = result;
    free(key);
}

static void expand_content(Archive *a, Visit *v, const Block *b, const char **clabels, int cn,
                           const char *where, Buf *out) {
    const char *content = b->content;
    size_t len = b->content_len;
    size_t i = 0;
    while (i < len) {
        if (content[i] == '@' && i + 1 < len) {
            unsigned char nch = (unsigned char)content[i + 1];
            if (nch == '@') {
                buf_append(out, "@", 1);
                i += 2;
                continue;
            }
            if (nch < 128 && !is_ascii_alpha(nch)) {
                Parser sub;
                QName qn;
                int explicit_labels;
                const char **use_l;
                int use_n;
                const char *use_m;
                StrVec resolved = {0};
                Buf piece = {0};
                Parser loc;
                if (nch != '<') {
                    Parser errp;
                    memset(&errp, 0, sizeof(errp));
                    errp.where = where;
                    errp.def_line = b->def_line;
                    errp.def_file = b->def_file;
                    fail_at(&errp, "unexpected control '@%c'", nch);
                }
                memset(&sub, 0, sizeof(sub));
                sub.buf = content;
                sub.n = len;
                sub.i = i + 2;
                sub.arc = a;
                sub.where = where;
                sub.def_line = b->def_line;
                sub.def_file = b->def_file;
                parse_qname(&sub, &qn, Q_REF);
                if (control_kind(&sub) != '>') {
                    fail_at(&sub, "expected '@>'");
                }
                explicit_labels = qn.labels.n > 0;
                if (explicit_labels) {
                    expand_label_list(a, qn.labels.v, qn.labels.n, &resolved);
                    use_l = resolved.v;
                    use_n = resolved.n;
                } else {
                    use_l = clabels;
                    use_n = cn;
                }
                use_m = qn.has_module ? qn.module : b->def_module;
                expand_ref(a, v, use_l, use_n, use_m, qn.chunk, &piece);
                memset(&loc, 0, sizeof(loc));
                loc.where = where;
                loc.def_line = b->def_line;
                loc.def_file = b->def_file;
                for (int fi = 0; fi < qn.nfilters; fi++) {
                    QName *f = &qn.filters[fi];
                    /* A filter with no labels keeps the labels of the
                       reference being expanded, not labels written on
                       the chunk reference it is attached to. */
                    run_filter(a, v, &piece, f->labels.v, f->labels.n, f->has_module, f->module,
                               f->chunk, clabels, cn, b->def_module, &loc);
                }
                buf_append(out, piece.data, piece.n);
                free(piece.data);
                free(resolved.v);
                qname_free(&qn);
                i = sub.i + 2;
                continue;
            }
        }
        {
            size_t j = i + 1;
            while (j < len) {
                if (content[j] == '@' && j + 1 < len) {
                    unsigned char nch = (unsigned char)content[j + 1];
                    if (nch < 128 && !is_ascii_alpha(nch)) {
                        break;
                    }
                }
                j++;
            }
            buf_append(out, content + i, j - i);
            i = j;
        }
    }
}

static int block_in_chain(Archive *a, const Block *b, const char **labels,
                          int nlabels, const char *module, const char *chunk) {
    StrVec expanded = {0};
    int match;
    if (strcmp(b->module, module) != 0 || strcmp(b->chunk, chunk) != 0) {
        return 0;
    }
    expand_label_list(a, b->labels, b->nlabels, &expanded);
    match = is_subsequence(expanded.v, expanded.n, labels, nlabels);
    free(expanded.v);
    return match;
}

static int chunk_defined(Archive *a, const char **labels, int nlabels,
                         const char *module, const char *chunk) {
    for (int i = 0; i < a->nblocks; i++) {
        if (block_in_chain(a, &a->blocks[i], labels, nlabels, module, chunk)) {
            return 1;
        }
    }
    return 0;
}

static void expand_ref(Archive *a, Visit *v, const char **labels, int nlabels,
                       const char *module, const char *chunk, Buf *out) {
    char *key = canonical(labels, nlabels, module, chunk);
    if (visit_has(v, key)) {
        fail("circular inclusion of '%s'", key);
    }
    visit_push(v, key);
    for (int i = 0; i < a->nblocks; i++) {
        Block *b = &a->blocks[i];
        Buf body = {0};
        Parser loc;
        if (!block_in_chain(a, b, labels, nlabels, module, chunk)) {
            continue;
        }
        /* References in the body are expanded first. The block's filters
           then transform that text, and the filter output is what this
           block contributes. The output is not scanned for references. */
        expand_content(a, v, b, labels, nlabels, key, &body);
        memset(&loc, 0, sizeof(loc));
        loc.where = key;
        loc.def_line = b->def_line;
        loc.def_file = b->def_file;
        for (int fi = 0; fi < b->nfilters; fi++) {
            Filt *f = &b->filters[fi];
            run_filter(a, v, &body, f->labels, f->nlabels, f->has_module, f->module, f->chunk,
                       labels, nlabels, b->def_module, &loc);
        }
        buf_append(out, body.data, body.n);
        free(body.data);
    }
    visit_pop(v);
}

static char *read_file(const char *path, size_t *out_n) {
    FILE *f = fopen(path, "rb");
    long sz;
    char *buf;
    size_t n;
    if (!f) {
        fail("could not open '%s': %s", path, strerror(errno));
    }
    if (fseek(f, 0, SEEK_END) != 0) {
        fclose(f);
        fail("could not read '%s'", path);
    }
    sz = ftell(f);
    if (sz < 0 || fseek(f, 0, SEEK_SET) != 0) {
        fclose(f);
        fail("could not read '%s'", path);
    }
    buf = xmalloc((size_t)sz + 1);
    n = fread(buf, 1, (size_t)sz, f);
    fclose(f);
    if (n != (size_t)sz) {
        free(buf);
        fail("could not read '%s'", path);
    }
    buf[n] = '\0';
    *out_n = n;
    return buf;
}

static int is_regular(const char *path) {
    struct stat st;
    if (stat(path, &st) != 0) {
        return 0;
    }
    return S_ISREG(st.st_mode);
}

static char *join_dir(const char *dir, const char *name) {
    size_t ld = strlen(dir);
    size_t ln = strlen(name);
    int slash = !(ld > 0 && dir[ld - 1] == '/');
    char *s = xmalloc(ld + (size_t)slash + ln + 1);
    memcpy(s, dir, ld);
    if (slash) {
        s[ld++] = '/';
    }
    memcpy(s + ld, name, ln + 1);
    return s;
}

static char *try_dir(const char *dir, const char *name) {
    char *path = join_dir(dir, name);
    if (is_regular(path)) {
        return path;
    }
    free(path);
    return NULL;
}

/* An empty $LITAR_INCLUDE entry means the current directory, as in $PATH. */

static char *search_include_list(const char *env, const char *name) {
    const char *s = env;
    while (*s) {
        const char *colon = strchr(s, ':');
        size_t len = colon ? (size_t)(colon - s) : strlen(s);
        char *dir;
        char *found;
        if (len == 0) {
            dir = xstrdup(".");
        } else {
            dir = xmalloc(len + 1);
            memcpy(dir, s, len);
            dir[len] = '\0';
        }
        found = try_dir(dir, name);
        free(dir);
        if (found) {
            return found;
        }
        if (!colon) {
            return NULL;
        }
        s = colon + 1;
        if (*s == '\0') {
            return try_dir(".", name);
        }
    }
    return NULL;
}

static char *find_library(const char *name) {
    char *found;
    const char *env;
    const char *home;
    if (name[0] == '/') {
        if (!is_regular(name)) {
            fail("could not find archive '%s'", name);
        }
        return xstrdup(name);
    }
    found = try_dir(".", name);
    if (found) {
        return found;
    }
    env = getenv("LITAR_INCLUDE");
    if (env) {
        found = search_include_list(env, name);
        if (found) {
            return found;
        }
    }
    home = getenv("HOME");
    if (home && home[0]) {
        char *dir = join_dir(home, ".litar");
        found = try_dir(dir, name);
        free(dir);
        if (found) {
            return found;
        }
        dir = join_dir(home, ".local/share/litar");
        found = try_dir(dir, name);
        free(dir);
        if (found) {
            return found;
        }
    }
    found = try_dir("/usr/local/share/litar", name);
    if (found) {
        return found;
    }
    found = try_dir("/usr/share/litar", name);
    if (found) {
        return found;
    }
    fail("could not find archive '%s'", name);
}

static void parse_buffer(Archive *a, const char *buf, size_t n, const char *filename) {
    Parser p;
    /* An include guard stops a second inclusion from repeating the body.
       A cycle with no guard would recurse without limit. */
    if (a->inc_depth >= 64) {
        fail("includes are nested too deeply in '%s'", filename);
    }
    a->inc_depth++;
    memset(&p, 0, sizeof(p));
    p.buf = buf;
    p.n = n;
    p.arc = a;
    p.report_lines = 1;
    p.filename = arena_strndup(&a->arena, filename, strlen(filename));
    parse_items(&p, 1, 0);
    a->inc_depth--;
}

/* @. name @ inserts that archive here. The included archive starts in
   the anonymous module, and the including archive keeps its own module.
   Chunks, flags, and label groups from the included archive remain. */

static void parse_include(Parser *p, int execute) {
    char *name;
    char *path;
    char *buf;
    size_t n = 0;
    const char *saved;
    p->i += 2;
    name = read_name_until_control(p);
    if (!name[0]) {
        fail_at(p, "empty archive name");
    }
    if (!is_end_kind(control_kind(p))) {
        fail_at(p, "expected '@' to end the expression");
    }
    consume_end(p);
    if (!execute) {
        return;
    }
    path = find_library(name);
    buf = read_file(path, &n);
    saved = p->arc->current_module;
    p->arc->current_module = p->arc->anonymous_module;
    parse_buffer(p->arc, buf, n, path);
    p->arc->current_module = saved;
    free(buf);
    free(path);
}

static void usage(FILE *fp, const char *argv0) {
    fprintf(fp,
            "Usage:\n"
            "  %s --help\n"
            "  %s -p EXPRESSION ARCHIVE\n"
            "  %s --print EXPRESSION ARCHIVE\n"
            "\n"
            "--help\n"
            "    print the usage\n"
            "-p, --print EXPRESSION\n"
            "    evaluate EXPRESSION, and print it\n"
            "\n"
            "EXPRESSION follows "
            "label1@:label2@:module name@/chunk name@|filter1@|filter2.\n",
            argv0, argv0, argv0);
}

int main(int argc, char **argv) {
    const char *argv0 = (argc > 0 && argv[0] && argv[0][0]) ? argv[0] : "litar";
    const char *expr = NULL;
    const char *file = NULL;
    Archive arc;
    Parser ep;
    QName qn;
    StrVec labels = {0};
    const char *module;
    Visit visit = {0};
    Buf result = {0};
    char *key;

    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "--help") == 0) {
            usage(stdout, argv0);
            return 0;
        }
    }
    for (int i = 1; i < argc; i++) {
        const char *arg = argv[i];
        if (strcmp(arg, "-p") == 0 || strcmp(arg, "--print") == 0) {
            if (expr) {
                fail("duplicate print option");
            }
            if (i + 1 >= argc) {
                usage(stderr, argv0);
                fail("%s requires an expression", arg);
            }
            expr = argv[++i];
        } else if (strncmp(arg, "--print=", 8) == 0) {
            if (expr) {
                fail("duplicate print option");
            }
            expr = arg + 8;
        } else if (arg[0] == '-' && arg[1] != '\0') {
            usage(stderr, argv0);
            fail("unknown option '%s'", arg);
        } else {
            if (file) {
                fail("unexpected argument '%s'", arg);
            }
            file = arg;
        }
    }
    if (!expr || !file) {
        usage(stderr, argv0);
        if (!expr) {
            fail("missing -p or --print");
        }
        fail("missing archive");
    }

    memset(&arc, 0, sizeof(arc));
    arc.src = read_file(file, &arc.src_n);
    arc.anonymous_module = arena_strndup(&arc.arena, "", 0);
    arc.current_module = arc.anonymous_module;
    parse_buffer(&arc, arc.src, arc.src_n, file);

    memset(&ep, 0, sizeof(ep));
    ep.buf = expr;
    ep.n = strlen(expr);
    ep.arc = &arc;
    parse_qname(&ep, &qn, Q_EXPR);
    if (ep.i < ep.n) {
        int k = control_kind(&ep);
        if (k == 0) {
            fail_at(&ep, "unterminated name");
        }
        fail_at(&ep, "unexpected control '@%c' in name", k);
    }
    if (qn.labels.n) {
        expand_label_list(&arc, qn.labels.v, qn.labels.n, &labels);
    }
    module = qn.has_module ? qn.module : "";
    key = canonical(labels.v, labels.n, module, qn.chunk);
    if (!chunk_defined(&arc, labels.v, labels.n, module, qn.chunk)) {
        fail("chunk '%s' is not defined", key);
    }
    free(key);
    /* The chunk is built in memory and written only after every filter
       has run, so a cycle or a failed filter leaves stdout empty and
       each filter runs once. */
    expand_ref(&arc, &visit, labels.v, labels.n, module, qn.chunk, &result);
    if (qn.nfilters) {
        Parser loc;
        memset(&loc, 0, sizeof(loc));
        for (int fi = 0; fi < qn.nfilters; fi++) {
            QName *f = &qn.filters[fi];
            /* A print expression is not inside a block, so a filter
               with no module is in the anonymous module. */
            run_filter(&arc, &visit, &result, f->labels.v, f->labels.n, f->has_module,
                       f->module, f->chunk, labels.v, labels.n, "", &loc);
        }
    }
    if (result.n && fwrite(result.data, 1, result.n, stdout) != result.n) {
        fail("could not write output");
    }
    free(result.data);

    free(labels.v);
    qname_free(&qn);
    free(visit.keys);
    free(arc.blocks);
    free(arc.src);
    arena_free(&arc.arena);
    return 0;
}
