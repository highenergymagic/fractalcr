#include "line-editor.h"
#include <assert.h>
#include <stdio.h>
#include <string.h>
static struct line_editor e;
static void type(const char *s) { while (*s) editor_feed(&e,(unsigned char)*s++); }
int main(void) {
    const char *framed="one\r\ntwo\rthree\n\r\n\n";
    char normalized[64]={0}; size_t used=0; int after_cr=0;
    while (*framed) {
        unsigned char c=(unsigned char)*framed++;
        if(terminal_input_byte(&c,&after_cr)) normalized[used++]=(char)c;
    }
    assert(!strcmp(normalized,"one\ntwo\nthree\n\n\n"));
    puts("PASS CR, LF and fragmented CRLF produce exactly one submission per line ending");
    editor_init(&e); type("sh");
    assert(editor_feed(&e,'?')==EDIT_HELP && !strcmp(e.line,"sh") && e.cursor==2);
    type("ow "); assert(editor_feed(&e,'?')==EDIT_HELP && !strcmp(e.line,"show "));
    editor_feed(&e,22); assert(editor_feed(&e,'?')==EDIT_INSERT && !strcmp(e.line,"show ?"));
    puts("PASS context help preserves input and Ctrl-V inserts a literal question mark");
    editor_clear(&e); type("show versin"); type("\033[D"); type("o"); assert(!strcmp(e.line,"show version"));
    editor_feed(&e,1); editor_feed(&e,4); assert(!strcmp(e.line,"how version"));
    type("s"); editor_feed(&e,5); type("x"); editor_feed(&e,127); assert(!strcmp(e.line,"show version"));
    puts("PASS cursor editing, insertion, delete, home/end and backspace");
    editor_record(&e); editor_clear(&e); type("show plat"); editor_feed(&e,16);
    assert(!strcmp(e.line,"show version")); editor_feed(&e,14); assert(!strcmp(e.line,"show plat"));
    puts("PASS history navigation restores the unfinished input draft");
    editor_clear(&e); type("one two three"); type("\033b"); assert(e.cursor==8);
    editor_feed(&e,21); assert(!strcmp(e.line,"three") && e.cursor==0);
    editor_feed(&e,25); assert(!strcmp(e.line,"one two three"));
    editor_feed(&e,5); editor_feed(&e,23); assert(!strcmp(e.line,"one two "));
    editor_feed(&e,25); assert(!strcmp(e.line,"one two three"));
    type("\033y"); assert(!strcmp(e.line,"one two one two "));
    puts("PASS word movement, Ctrl-U/Ctrl-W deletion and kill-ring recall");
    editor_clear(&e);type("  fIRst SECOND third");editor_feed(&e,1);
    type("\033c");assert(!strcmp(e.line,"  First SECOND third") && e.cursor==7);
    type("\033L");assert(!strcmp(e.line,"  First second third") && e.cursor==14);
    type("\033u");assert(!strcmp(e.line,"  First second THIRD") && e.cursor==20);
    editor_feed(&e,1);type("\033D");assert(!strcmp(e.line," second THIRD") && e.cursor==0);
    editor_feed(&e,25);assert(!strcmp(e.line,"  First second THIRD"));
    editor_feed(&e,1);editor_feed(&e,6);editor_feed(&e,6);editor_feed(&e,6);
    type("\033u");assert(!strcmp(e.line,"  FIRST second THIRD") && e.cursor==7);
    editor_feed(&e,5);type("\033d\033c\033u\033l");assert(e.cursor==e.length);
    puts("PASS escape word deletion, kill-ring recall and case conversions handle spaces, partial words and end of line");
    editor_clear(&e); type("ab"); editor_feed(&e,20); assert(!strcmp(e.line,"ba"));
    assert(editor_feed(&e,3)==EDIT_CANCEL && !e.length);
    type("draft"); assert(editor_feed(&e,26)==EDIT_END && !e.length);
    assert(editor_feed(&e,4)==EDIT_EOF && !e.length);
    puts("PASS transpose, cancellation and return to EXEC");
    e.editing=0; type("abc");
    assert(editor_feed(&e,1)==EDIT_BELL && e.cursor==3);
    assert(editor_feed(&e,127)==EDIT_ERASE && !strcmp(e.line,"ab"));
    assert(editor_feed(&e,'\n')==EDIT_SUBMIT);
    e.editing=1; assert(editor_feed(&e,1)==EDIT_REDRAW && e.cursor==0);
    editor_clear(&e);
    puts("PASS disabled enhanced editing retains basic input and can be reenabled");
    for (unsigned i=0;i<LINE_CAPACITY-1;i++) editor_feed(&e,'x');
    assert(editor_feed(&e,'x')==EDIT_BELL && e.overflow && e.length==LINE_CAPACITY-1);
    editor_feed(&e,127); assert(!e.overflow && e.length==LINE_CAPACITY-2);
    puts("PASS bounded input rejects overflow and permits correction");
    editor_init(&e);editor_history_limit(&e,2);
    type("first");editor_record(&e);editor_clear(&e);
    type("second");editor_record(&e);editor_clear(&e);
    type("third");editor_record(&e);editor_clear(&e);
    editor_history_limit(&e,3);
    editor_feed(&e,16);assert(!strcmp(e.line,"third"));
    editor_feed(&e,16);assert(!strcmp(e.line,"second"));
    assert(editor_feed(&e,16)==EDIT_BELL);
    puts("PASS increasing history size cannot resurrect commands evicted from a smaller buffer");
    return 0;
}
