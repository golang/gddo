// Copyright 2012 Gary Burd
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package database

import (
	"strings"
)

var stopWord = createStopWordMap()

func createStopWordMap() map[string]bool {
	m := make(map[string]bool)
	for _, s := range strings.Fields(stopText) {
		m[s] = true
	}
	return m
}

const stopText = `
a
about
after
all
also
am
an
and
another
any
are
as
at
b
be
because
been
before
being
between
both
but
by
c
came
can
come
could
d
did
do
e
each
f
for
from
g
get
got
h
had
has
have
he
her
here
him
himself
his
how
i
if
implement
implements
in
into
is
it
j
k
l
like
m
make
many
me
might
more
most
much
must
my
n
never
now
o
of
on
only
or
other
our
out
over
p
q
r
s
said
same
see
should
since
some
still
such
t
take
than
that
the
their
them
then
there
these
they
this
those
through
to
too
u
under
v
w
x
y
z
`
