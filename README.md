# go-bc5
A golang implementation of the BC5 red/green image compression technique.

## Overview
This library allows you to compress and decompress RGBA image data to and from BC5 encoded blocks. It also includes functionality for writing and reading BC5 encoded data to/from an `io.Writer` or `io.Reader`.

BC5 data encoded using `*BC5.Encode(w io.Writer)` will write a 12-byte header at the beginning of the stream, containing the uint32 equivalent of `"BC5 "` encoded in Big Endian format (0x42433520) followed by two uint32 values denoting the width and height of the image. The proceeding byte is the start of the block data and continues until EOF.  In addition, `*BC5.Decode(r io.Reader)` expects the header and will error if it is not present.

The image on the left is the original, and the image on the right has been compressed and decompressed. The blue value difference is due to the original not being normalised.
![Before and after](https://imgur.com/xDj4yie)

### About BC5
BC5 is a two-channel image compression format where the red and green components in a 4x4 block of pixels are mapped to interpolated colour values between two stored reference colours. Each pixel in the 4x4 grid is assigned a 3-bit index stored along with the reference colours to be interpolated at runtime. The full spec for the BC5 format can be found in the MSDN documentation here: https://docs.microsoft.com/en-gb/windows/win32/direct3d10/d3d10-graphics-programming-guide-resources-block-compression#bc5 

The format is ideal for heightmaps or tangent-space normal maps where the blue component of each pixel can be reconstructed without much data loss.

For greyscale heightmaps, BC4 may produce smaller output files as only one channel is really needed. More sophisticated tools may gain the benefit of having an extra channel to work with when using over BC4 however, so it has its uses.

BC5 is ideally used for normal maps where only the x (red) and y (green) normal components need to be sampled by a shader, and the z (blue) is reconstructed with the assumption the original image was normalized.

## Notice
This is an early attempt at implementing the raw BC5 compression/decompression algorithm. Once any header is removed, the data format _should_ be acceptable to OpenGL using the `COMPRESSED_RG_RGTC2` format. I have not tested this however, so use at your own risk and feel free to contact me if you find any inconsistencies with the specification. 

## TODO
* Test with OpenGL.
* Improve API for dealing with the header. Allow the programmer to specify their own for writing and a func interface for parsing them.
