# Injects a bold @font-face (JetBrains Mono Bold, woff2) after the regular one
# freeze embeds, so font-weight="bold" spans use a face with identical metrics.
# Inject a bold @font-face (JetBrains Mono Bold, woff2) after freeze's regular one.
import base64, sys
p, woff = sys.argv[1], sys.argv[2]
s = open(p).read()
b64 = base64.b64encode(open(woff, 'rb').read()).decode()
face = ("@font-face {\n\tfont-family: &apos;JetBrains Mono&apos;;\n\tsrc: url(data:application/x-font-woff2;charset=utf-8;base64,%s) format(&apos;woff2&apos;);\n\tfont-weight: bold;\n\tfont-style: normal;\n}\n" % b64)
i = s.index('</style>')
open(p, 'w').write(s[:i] + face + s[i:])
