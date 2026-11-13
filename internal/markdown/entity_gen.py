"""Write entity.go: the named character references of HTML 4.01 (the 252
entities of its section 24, as Python's html.entities lists them) and
"apos", which XML and HTML5 added.

usage: python3 -I entity_gen.py | gofmt > entity.go
"""
import html.entities

names = dict(html.entities.name2codepoint)
names["apos"] = 0x27
print("package markdown")
print()
print("// entities maps the name of a character reference, as in \"&amp;\", to the")
print("// text it stands for. These are the 252 named references of HTML 4.01")
print("// (section 24 of the W3C recommendation) and \"apos\". The table was written")
print("// out by entity_gen.py from the list in Python's html.entities module. A")
print("// name that is not here is shown as it was written.")
print("var entities = map[string]string{")
for name in sorted(names):
    cp = names[name]
    print('\t"%s": "\\u%04x",' % (name, cp))
print("}")
