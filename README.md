[![Go CI](https://github.com/gregyjames/readr/actions/workflows/go.yml/badge.svg)](https://github.com/gregyjames/readr/actions/workflows/go.yml)
![Docker Image Size (tag)](https://img.shields.io/docker/image-size/gjames8/readr-frontend/latest?label=Frontend)
![Docker Image Size (tag)](https://img.shields.io/docker/image-size/gjames8/readr-backend/latest?label=Backend)

# Readr
A minimal, self hosted read it later app written powered by Vue/Tailwind and Go. All documents are stored as markdown files in a shared volume and tracked by the backend in a SQLLite database. Why? Because Pocket screwed me over **one** too many times. 

## Features
- Fast and fairly lightweight (53mb frontend/5mb backend)
- Automatically save and convert articles and their images as markdown
- Beautiful typography and syntax hilighting (imo)

## Todo
- [ ] Add proxy for backend so it's not hardcoded (do not change port)
- [ ] Dark mode (should have been the first thing I added, do I even code bro)
  - [ ] I think my tailwind usage **might** be f***** (when dark set to class, it's always showing up) so I need to fix that first.
- [ ] Make more responsive (the children long for the responsive grids)
- [ ] Image compression?
- [ ] Authentication? Protect endpoints? 
- [ ] Add an edit mode for tags/article details

## Sample
<table>
  <tr>
    <td>
      <img src="https://github.com/gregyjames/readr/blob/main/samples/home.png?raw=true" width="750px"/>
    </td>
    <td>
      <img src="https://github.com/gregyjames/readr/blob/main/samples/article.png?raw=true" width="750px"/>
    </td>
  </tr>
</table>

## Docker Compose
```yaml
version: '3.8'

services:
  frontend:
    image: "gjames8/readr-frontend:latest"
    ports:
      - "8080:8080"
    container_name: frontend
  backend:
    image: "gjames8/readr-backend:latest"
    ports:
      - "3000:3000"
    container_name: backend
    volumes:
      - ./data:/app/data
```

## Design choices
### Why markdown?
Yeah it probably would have been easier to just save them as HTML files but I have a script that syncs these with my obsidian vault, so I needed these to be markdown. 
### Why is the design so meh?
WELL I LIKE IT ಥ‿ಥ this is what happens when you let backend guys work on GUIs you guys are lucky I didn't make this a CLI app. In all seriousness though, I was going for a very lightweight minimal design so if you see something that needs to be fixed or can be improved please feel free to open a PR! 

## Limitations
1. I am currently kinda hard coding that route to the backend (bad) so it will _not work_ without it running on port 3000. Need to set up a proxy in the frontend to handle all of that.
2. Not every article will convert perfectly, depending on the design of the website.

## License
MIT License

Copyright (c) 2024 Greg James

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
