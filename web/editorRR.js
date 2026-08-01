
const PASSWORD_WORDS_PATH = '/passwordwords.md';

async function generatePassword() {
    try {
        const words = await loadPasswordWords();


        const password = await GenPass(words);

        // Insert into editor at cursor or replace selection
        const cm = currentEditor || editor;
        if (cm) {
            cm.replaceSelection(password);
            cm.focus();
        } else {
            // Fallback: copy to clipboard
            await navigator.clipboard.writeText(password);
        }
        showToast('Password generated');
    } catch (err) {
        logError('generatePassword:', err);
        showToast('Failed: ' + err.message);
    }
}

async function GenPass(pWords) {
    var thePass = "";

    var words = ['Bacon', 'Eggs', 'Cosmos', 'Spike'
        , 'Sage', 'Sweet', 'Thing', 'Horse', 'Battery'
        , 'Staple', 'Jiggy', 'Dino', 'Truck', 'Monster'
        , 'Bananna', 'Chilli', 'Dog', 'Cat', 'Bird'
        , 'Wind', 'Planet', 'Fire', 'Earth', 'Moon'
        , 'Razor', 'Zebra', 'Lion', 'Charlie', 'Tango'
        , 'Sofa', 'Table', 'Phone', 'Plant', 'Pinapple'
        , 'Beer', 'Wine', 'Potato', 'Crackle', 'Snap', 'Pop'
        , 'Funny', 'Sugar', 'Happy', 'Fork', 'Big', 'Flock', 'Shape'


    ];

    if (pWords) {
        words = pWords;
    }
    var maxwords = 3;



    const rndarray = new Uint8Array(20);
    self.crypto.getRandomValues(rndarray);

    //Pick Words
    for (let i = 0; i < maxwords; i++) {
        var pick = rndarray[i] % words.length;
        thePass += await LettersReplace(words[pick]);
        // remove the pick so it can not be picked again
        words.splice(pick, 1);
    }

    // add numbers
    thePass += "." + ((rndarray[4] % 90) + 9);



    return thePass

}

async function LettersReplace(text) {
    var ReplacedCountTotal = 0;
    const rndarray = new Uint8Array(20);
    self.crypto.getRandomValues(rndarray);

    var letters = ['o', 'e', 'S', 'a', 't', 'i', 'c', 'k'];
    var letreps = ['0', '3', '$', '@', '+', '!', '(', '<'];
    // replace letters
    var ReplaceCount = 0
    var letterindex = [];
    for (let i = 0; i < letters.length; i++) {
        if (text.indexOf(letters[i]) > -1) {
            ReplaceCount += 1;
            letterindex.push(i);
        }
    }

    if (ReplaceCount > 0) {
        // pick one to replace
        var pick = (rndarray[5] % ReplaceCount);
        var indexpick = letterindex[pick];
        text = text.replace(letters[indexpick], letreps[indexpick]);
        // remove it from the list so it can not use the same replace thing again
        letters.splice(indexpick, 1);
        letreps.splice(indexpick, 1);
    }

    ReplacedCountTotal += ReplaceCount;
    return text;
}

async function loadPasswordWords() {
    const memFile = getMemFile(PASSWORD_WORDS_PATH);
    if (!memFile) {
        return null
        //throw new Error('passwordwords.md not found');
    }

    let content;
    if (memFile.handle instanceof FileSystemFileHandle) {
        const file = await memFile.handle.getFile();
        content = await file.text();
    } else if (memFile.content !== undefined) {
        content = memFile.content;
    } else {
        throw new Error('Cannot read passwordwords.md');
    }

    // Parse words: one per line, skip the header and blank lines
    const words = content.split('\n')
        .map(line => line.replace(/^#\s*/, '').trim())
        .filter(line => line.length > 0 && !line.startsWith('#'));

    if (words.length === 0) {
        throw new Error('No words found in passwordwords.md');
    }
    return words;
}

function pickRandomWords(words, count) {
    const picked = [];
    const len = words.length;
    for (let i = 0; i < count; i++) {
        const idx = crypto.getRandomValues(new Uint32Array(1))[0] % len;
        picked.push(words[idx]);
    }
    return picked;
}

async function generatePasswordOLD() {
    try {
        const words = await loadPasswordWords();
        // Ask user how many words (default 4)
        const countInput = prompt('Number of words in password:', '4');
        const count = parseInt(countInput, 10);
        if (isNaN(count) || count < 1) {
            showToast('Invalid word count');
            return;
        }

        // Ask for separator
        const sepInput = prompt('Separator between words:', '-');
        const sep = sepInput !== null ? sepInput : '-';

        // Ask for capitalization style
        const capInput = prompt('Capitalization? (lower / Upper / UPPER / number)', 'Upper');
        let capStyle = 'Upper';
        if (['lower', 'upper', 'number'].includes(capInput)) {
            capStyle = capInput;
        }

        const picked = pickRandomWords(words, count);
        const transformed = picked.map(word => {
            switch (capStyle) {
                case 'lower': return word.toLowerCase();
                case 'upper': return word.toUpperCase();
                case 'number': return word.charAt(0).toUpperCase() + word.slice(1).toLowerCase() + Math.floor(Math.random() * 10);
                default: // Upper
                    return word.charAt(0).toUpperCase() + word.slice(1).toLowerCase();
            }
        });

        const password = transformed.join(sep);

        // Insert into editor at cursor or replace selection
        const cm = currentEditor || editor;
        if (cm) {
            cm.replaceSelection(password);
            cm.focus();
        } else {
            // Fallback: copy to clipboard
            await navigator.clipboard.writeText(password);
        }
        showToast('Password generated');
    } catch (err) {
        logError('generatePassword:', err);
        showToast('Failed: ' + err.message);
    }
}