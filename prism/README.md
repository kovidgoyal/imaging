This code is based on code from two other projects,
https://github.com/mandykoh/prism (mainly image file format parsing, MIT license)
and http://github.com/go-andiamo/iccarus (mainly ICC file parsing, Apache 2.0 license)

The code from these is modified and optimised and fixed with a large number of
changes. 

It is imported so that optimised conversion routes for imaging.NRGB can be
written, which would cause a circular dependency otherwise.
