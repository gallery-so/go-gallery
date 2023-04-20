# Gallery Pseudo-Lang

## Context:

Generate a gallery of media items on a profile page based on user input and available media. Media items have unique IDs, names, and group names. Organize the data into rows, columns, and collections.

Inputs:

1. User's prompt: text determining gallery organization (e.g., "put all cat NFTs in one collection with 3 columns" or "create a seemingly random gallery")
2. Semi-colon separated list of media items (comma-separated ID, name, group name), e.g.:

```
12,Autoglyph #1,Autoglyphs;205,Autoglyph #22,Autoglyphs;124,Autoglyph #523,Autoglyphs;4,Autoglyph #204,Autoglyphs;5,Autoglyph #2042,Autoglyphs;6,Doodle #1,The Doodles;7,Doodle #2912,The Doodles;8,Death,Feelings;9,Happy,Feelings;10,Dispair,Feelings
```

## Output Format:

JSON object structure:

1. Parentheses: collections
2. [: row start
3. ]: row end
4. Commas: separate items in rows and rows themselves

The output will adhere to the following rules:

- Every row has a collection
- A row can have between 1 and 6 items

Example:

```
([12,205,124],[4,5]),([6,7]),([8,9,10])
```

Input format as a single string:

```
{prompt}:{tokens}
```

## Example:

Input:

```
Organize my NFTs into collections based on the NFT's collection:12,Autoglyph #1,Autoglyphs;205,Autoglyph #22,Autoglyphs;124,Autoglyph #523,Autoglyphs;4,Autoglyph #204,Autoglyphs;5,Autoglyph #2042,Autoglyphs;6,Doodle #1,The Doodles;7,Doodle #2912,The Doodles;8,Death,Feelings;9,Happy,Feelings;10,Dispair,Feelings
```

Output:

```
([12,205,124],[4,5]),([6,7]),([8,9,10])
```