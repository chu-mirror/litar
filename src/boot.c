/* Bootstrap of litar: the archive syntax through Including.
   Filters, files, and file sets belong to later stages.
   See design/litar.md and design/stages.md. */

#include <errno.h>
#include <stdarg.h>
#include <stddef.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>

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
} QName;

typedef struct Block {
    const char **labels;
    int nlabels;
    const char *module;
    const char *chunk;
    const char *content;
    size_t content_len;
    int def_line;
    const char *def_file;
    const char *def_module;
} Block;

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
   ends on '@', which closes an expression. -1 means this is not a control. */

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

static char *take_name(Parser *p, size_t start, size_t end) {
    while (start < end && is_trim_space((unsigned char)p->buf[start])) {
        start++;
    }
    while (end > start && is_trim_space((unsigned char)p->buf[end - 1])) {
        end--;
    }
    return arena_strndup(&p->arc->arena, p->buf + start, end - start);
}

static char *read_name_until_control(Parser *p) {
    size_t start = p->i;
    while (p->i < p->n && control_kind(p) < 0) {
        p->i++;
    }
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

static void parse_qname(Parser *p, QName *qn, int mode) {
    memset(qn, 0, sizeof(*qn));
    for (;;) {
        size_t start = p->i;
        int k;
        char *name;
        while (p->i < p->n && control_kind(p) < 0) {
            p->i++;
        }
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
            fail_at(p, "filters are not implemented");
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
    while (p->i < p->n && control_kind(p) < 0) {
        p->i++;
    }
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

static void parse_block(Parser *p, int execute) {
    QName qn;
    Block blk;
    size_t body;
    size_t end;
    int line = line_at(p, p->i);
    p->i += 2;
    parse_qname(p, &qn, Q_BLOCK);
    if (control_kind(p) != '=') {
        fail_at(p, "expected '@='");
    }
    p->i += 2;
    body = p->i;
    while (p->i < p->n) {
        int k = control_kind(p);
        if (k >= 0 && is_end_kind(k)) {
            break;
        }
        p->i++;
    }
    if (p->i >= p->n) {
        fail_at(p, "unterminated block");
    }
    /* The newline after '@=' and the newline before the closing '@'
       are part of the block layout used throughout the design.
       @<msg@=\nHello\n@ therefore stores Hello. */
    end = p->i;
    if (body < end && p->buf[body] == '\r') {
        body++;
    }
    if (body < end && p->buf[body] == '\n') {
        body++;
    }
    if (end > body && p->buf[end - 1] == '\n') {
        end--;
        if (end > body && p->buf[end - 1] == '\r') {
            end--;
        }
    } else if (end > body && p->buf[end - 1] == '\r') {
        end--;
    }
    consume_end(p);
    if (!execute) {
        free(qn.labels.v);
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
    blk.def_line = line;
    blk.def_file = p->filename;
    blk.def_module = p->arc->current_module;
    free(qn.labels.v);
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
        if (k == '|') {
            fail_at(p, "filters are not implemented");
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

static void expand_ref(Archive *a, Visit *v, const char **labels, int nlabels,
                       const char *module, const char *chunk, int emit);

static void expand_content(Archive *a, Visit *v, const Block *b,
                           const char **clabels, int cn, const char *where,
                           int emit) {
    const char *content = b->content;
    size_t len = b->content_len;
    size_t i = 0;
    while (i < len) {
        if (content[i] == '@' && i + 1 < len) {
            unsigned char nch = (unsigned char)content[i + 1];
            if (nch < 128 && !is_ascii_alpha(nch)) {
                Parser sub;
                QName qn;
                int explicit_labels;
                const char **use_l;
                int use_n;
                const char *use_m;
                StrVec resolved = {0};
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
                expand_ref(a, v, use_l, use_n, use_m, qn.chunk, emit);
                free(qn.labels.v);
                free(resolved.v);
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
            if (emit && fwrite(content + i, 1, j - i, stdout) != j - i) {
                fail("could not write output");
            }
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
                       const char *module, const char *chunk, int emit) {
    char *key = canonical(labels, nlabels, module, chunk);
    if (visit_has(v, key)) {
        fail("circular inclusion of '%s'", key);
    }
    visit_push(v, key);
    for (int i = 0; i < a->nblocks; i++) {
        Block *b = &a->blocks[i];
        if (!block_in_chain(a, b, labels, nlabels, module, chunk)) {
            continue;
        }
        expand_content(a, v, b, labels, nlabels, key, emit);
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
    const char *argv0 = (argc > 0 && argv[0] && argv[0][0]) ? argv[0] : "stage0";
    const char *expr = NULL;
    const char *file = NULL;
    Archive arc;
    Parser ep;
    QName qn;
    StrVec labels = {0};
    const char *module;
    Visit visit = {0};
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
    if (qn.labels.n) {
        expand_label_list(&arc, qn.labels.v, qn.labels.n, &labels);
    }
    module = qn.has_module ? qn.module : "";
    key = canonical(labels.v, labels.n, module, qn.chunk);
    if (!chunk_defined(&arc, labels.v, labels.n, module, qn.chunk)) {
        fail("chunk '%s' is not defined", key);
    }
    free(key);
    /* Walk once before writing, so a cycle or a bad reference exits
       with empty stdout. */
    expand_ref(&arc, &visit, labels.v, labels.n, module, qn.chunk, 0);
    expand_ref(&arc, &visit, labels.v, labels.n, module, qn.chunk, 1);

    free(labels.v);
    free(qn.labels.v);
    free(visit.keys);
    free(arc.blocks);
    free(arc.src);
    arena_free(&arc.arena);
    return 0;
}
