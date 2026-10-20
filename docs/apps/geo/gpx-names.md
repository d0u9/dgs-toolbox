# Names in other languages in GPX

This is the format dgs uses to name a GPX file and its parts in more than one
language. It is a GPX 1.1 extension: a file that uses it is still valid GPX
1.1, and a reader that does not know it reads the file as before.

The key words MUST, MUST NOT, SHOULD and MAY are used as in RFC 2119.

## Why an extension

GPX 1.1 gives `<name>` one string and no language attribute. A name in a
second language therefore cannot go into `<name>` without breaking what other
readers show: `西湖 / West Lake` is long and is cut off on devices. The
extension keeps `<name>` as it is and puts each other language beside it. It
also says which language `<name>` itself is in, which GPX cannot say either:
a reader falling back to `<name>` needs it to mark the text's language, so
the right fonts are chosen, and to tell a missing translation from a name
that is already in the wanted language.

## The elements

| | Translated name | Language of `<name>` |
| --- | --- | --- |
| Namespace | `urn:dgs-toolbox:gpx:1` | `urn:dgs-toolbox:gpx:1` |
| Element | `name` | `lang` |
| Attribute | `xml:lang`, required: a BCP 47 language tag such as `en`, `zh`, `zh-Hant` or `ja` | none |
| Content | The name in that language: text only, one line | A BCP 47 language tag |
| How many | Any, one per language | At most one |

The namespace is the one dgs uses for its other GPX extensions. The prefix is
not part of the format; dgs writes `dgs` and declares it on each element.

## The language of `<name>`

A `<dgs:lang>` in an element's `<extensions>` says which language that
element's `<name>` is in. The one in `<metadata>` says it for the file's
`<name>`, and is also the default for every `<trk>`, `<rte>` and `<wpt>`
without its own. So the language of an element's `<name>` is:

1. its own `<dgs:lang>`;
2. else, for a `<trk>`, `<rte>` or `<wpt>`, the one in `<metadata>`;
3. else not known.

When it is not known, a reader MUST NOT assume one: not the language of the
file's other names, of the reader, or one guessed from the script. A file
that mixes languages, such as English names for some places and Chinese for
others, marks the exceptions on their elements.

## Where it goes

A translated name belongs to the element whose own `<name>` it translates.
It goes in that element's own `<extensions>`, which GPX 1.1 places as
follows:

| Element | Its `<extensions>` |
| --- | --- |
| `<metadata>` — the file | last child of `<metadata>` |
| `<trk>` — a track | after `<name>` and the other track children, before the first `<trkseg>` |
| `<rte>` — a route | after `<name>` and the other route children, before the first `<rtept>` |
| `<wpt>` — a waypoint | last child of `<wpt>` |

`<dgs:lang>` goes in the same `<extensions>`. `<trkseg>`, `<trkpt>` and
`<rtept>` take neither. The root `<gpx>` element's own `<extensions>` does
not either; the file is named in `<metadata>`.

An `<extensions>` MAY hold other extensions beside translated names and
`<dgs:lang>`, in any order.

## What is translated

Only `<name>` is. Every translated name stands for the `<name>` of the
element it is in, and for nothing else:

- `<desc>` and `<cmt>` of any element, `<wpt>` included, are not translated
  and are left as they are. A translated name says nothing about them, and a
  writer MUST NOT put a description into a translated name.
- A `<rte>` and its `<rtept>` elements are separate: the route's translated
  names translate the route's `<name>`, and a `<rtept>`'s `<name>` has none.
- Elements are translated each on its own. A route planned along a track, or
  a waypoint on it, does not take the track's translations, and changing one
  element's translations changes no other's.

## Tracks with several segments

A `<trk>` is translated as a whole: its translated names sit in the `<trk>`'s
own `<extensions>`, before the first `<trkseg>`, and name every segment of it
alike. A `<trkseg>` has no `<name>` in GPX 1.1, so it has nothing to
translate, and a reader MUST NOT look for translated names inside one.

A writer that splits a track into several `<trk>` elements, or joins several
into one, decides each new track's names afresh: the format does not say
which translation a part of a split track keeps, or which of the joined
tracks' translations the joined one takes. Copying a whole track keeps them.

## Keeping translations current

A translated name is not tied to the text of `<name>`: nothing in the file
says which `<name>` it was made from. When a writer changes `<name>`, it
SHOULD offer the translations to be changed at the same time, and MUST NOT
silently keep a translation it knows no longer fits. A reader cannot tell a
stale translation from a current one and shows it as it is.

## Example

```xml
<?xml version="1.0" encoding="UTF-8"?>
<gpx version="1.1" creator="dgs-toolbox" xmlns="http://www.topografix.com/GPX/1/1">
  <metadata>
    <name>杭州</name>
    <extensions>
      <dgs:lang xmlns:dgs="urn:dgs-toolbox:gpx:1">zh</dgs:lang>
      <dgs:name xmlns:dgs="urn:dgs-toolbox:gpx:1" xml:lang="en">Hangzhou</dgs:name>
    </extensions>
  </metadata>
  <wpt lat="30.2468000" lon="120.1454000">
    <name>断桥</name>
    <extensions>
      <dgs:name xmlns:dgs="urn:dgs-toolbox:gpx:1" xml:lang="en">Broken Bridge</dgs:name>
      <dgs:name xmlns:dgs="urn:dgs-toolbox:gpx:1" xml:lang="ja">断橋</dgs:name>
    </extensions>
  </wpt>
  <wpt lat="30.2590000" lon="120.1380000">
    <name>Hangzhou Hotel</name>
    <extensions><dgs:lang xmlns:dgs="urn:dgs-toolbox:gpx:1">en</dgs:lang></extensions>
  </wpt>
  <trk>
    <name>环湖</name>
    <extensions><dgs:name xmlns:dgs="urn:dgs-toolbox:gpx:1" xml:lang="en">Around the lake</dgs:name></extensions>
    <trkseg>
      <trkpt lat="30.2468000" lon="120.1454000"></trkpt>
    </trkseg>
  </trk>
</gpx>
```

## Writers

A writer:

- MUST keep `<name>`, and SHOULD keep it in the name's own language. It is
  what every reader without the extension shows, and what a reader with the
  extension falls back to.
- MUST write at most one translated name per language tag on one element.
- MUST NOT write a translated name with an empty text or without `xml:lang`.
- MUST NOT write a line break in a translated name. Leading and trailing white
  space is not part of the name.
- SHOULD NOT repeat `<name>` as a translation in the same language.
- SHOULD write the elements in one fixed order, so the same names make the
  same bytes. dgs writes `<dgs:lang>` first, then the translated names by
  tag.
- SHOULD say the language of `<name>` when it knows it: once in `<metadata>`
  for the file, and on an element only where it differs.
- MUST NOT write more than one `<dgs:lang>` in one `<extensions>`, or one
  that is empty.

## Readers

A reader:

- MUST take only `name` elements in the namespace `urn:dgs-toolbox:gpx:1`
  that are direct children of the element's own `<extensions>`. A `name` in
  another namespace is not a translation.
- MUST ignore a translated name without `xml:lang` or with empty text, after
  trimming white space.
- SHOULD take the last one when one element has several for the same tag.
- MUST take the language of `<name>` only from `<dgs:lang>` in the
  namespace, as *The language of `<name>`* says; with several in one
  `<extensions>`, the last, and an empty one is not there.
- MUST treat language tags as case-insensitive when it matches them, as
  BCP 47 does. dgs writes tags as given.

To show a name in a wanted language, a reader SHOULD choose among the
candidates: every translated name, each in its tag's language, and `<name>`
too when its language is known. It takes, in this order:

1. the candidate whose language is the wanted tag;
2. a candidate whose language starts with the wanted tag and `-`, so `zh`
   finds `zh-Hant`; when several do — `zh-Hans` and `zh-Hant` — the first
   by tag, compared in lower case character by character, so `zh-Hans`;
3. `<name>`, as a fallback.

When `<name>` and a translation have the same tag, `<name>` is taken. A name
taken in steps 1 and 2 is in the wanted language, or one of its regions or
scripts; one taken in step 3 is not translated into it, and when its language
is not known a reader MUST NOT mark the text with one.

A reader MAY also fall back from a longer wanted tag to a shorter one
(`zh-Hant-TW`, then `zh-Hant`, then `zh`) before step 3.

## Schema

The element as an XML Schema, for validating the extension on its own. GPX
1.1 lets `<extensions>` hold elements of any other namespace, so the GPX
schema accepts it as it is.

```xml
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
           targetNamespace="urn:dgs-toolbox:gpx:1"
           elementFormDefault="qualified">
  <xs:import namespace="http://www.w3.org/XML/1998/namespace"
             schemaLocation="http://www.w3.org/2001/xml.xsd"/>
  <xs:element name="name">
    <xs:complexType>
      <xs:simpleContent>
        <xs:extension base="xs:string">
          <xs:attribute ref="xml:lang" use="required"/>
        </xs:extension>
      </xs:simpleContent>
    </xs:complexType>
  </xs:element>
  <xs:element name="lang" type="xs:language"/>
</xs:schema>
```

## In dgs

- Only a GPX dgs wrote is written with translated names. A recording keeps
  its translations in its sidecar until it is saved as a new GPX; see
  [GPX](gpx.md#names-in-other-languages).
- dgs accepts tags of the form `^[A-Za-z]{2,8}(-[A-Za-z0-9]{1,8})*$`; whether
  a tag is registered is not checked.
- *Rename…* asks for `<name>`, its language and one translation together,
  so a renamed part's translation is in front of the user. Renaming the GPX
  sets the file's language, the default; renaming a part sets its own, and
  left empty the part takes the file's. *Restore the name* gives the GPX
  back its own `<name>`, language and translations together, so a
  translation given after a rename does not outlive the name it translated.
- A part copied into another GPX takes the language its name was in as its
  own, since the other file's default may be another — unless that is the
  other file's default, which it then takes, writing none of its own.
- dgs's segments are cuts it keeps in its own state, not `<trkseg>`
  elements, and their names have no translations. Writing segments into a
  new GPX, or joining tracks, makes tracks named afresh, without
  translations. *Copy to…*, *Move to…* and *Save as…* carry every
  translation with its element.
- Descriptions are not translated.
