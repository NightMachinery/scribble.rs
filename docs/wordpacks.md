# Wordpacks

Scribble.rs wordpacks live in `./wordpacks/`.

Each wordpack is a UTF-8 text file with one drawable word or phrase per line.
Use newline separation, not comma separation:

```text
apple
red balloon
space station
```

At startup, Scribble.rs loads the built-in files embedded from `wordpacks/` and
then scans the runtime `./wordpacks/` directory. Any regular text file in that
directory becomes a selectable wordpack. Files ending in `.txt` or `.text` use
the filename without the extension as the wordpack key; files without those
extensions use the full filename.

Runtime files with the same wordpack key as a built-in file override the
embedded version.
